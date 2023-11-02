/*
Copyright 2023 VMware Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"net/url"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/garethjevans/monorepository-controller/internal/monorepo"
	"github.com/jenkins-x/go-scm/scm/factory"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	apiv1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/garethjevans/monorepository-controller/api/v1alpha1"
	"github.com/garethjevans/monorepository-controller/internal/util"
	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories/finalizers,verbs=update
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=patch;create;update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func NewMonoRepositoryReconciler(c reconcilers.Config) *reconcilers.ResourceReconciler[*v1alpha1.MonoRepository] {
	return &reconcilers.ResourceReconciler[*v1alpha1.MonoRepository]{
		Name: "MonoRepository",
		Reconciler: reconcilers.Sequence[*v1alpha1.MonoRepository]{
			NewMonoRepositoryChildReconciler(c),
			NewRequeueReconciler(c),
		},
		Config: c,
	}
}

func NewRequeueReconciler(c reconcilers.Config) reconcilers.SubReconciler[*v1alpha1.MonoRepository] {
	return &reconcilers.SyncReconciler[*v1alpha1.MonoRepository]{
		Name: "Requeue",
		SyncWithResult: func(ctx context.Context, resource *v1alpha1.MonoRepository) (reconcilers.Result, error) {
			return reconcilers.Result{Requeue: true, RequeueAfter: resource.Spec.Interval.Duration}, nil
		},
	}
}

func NewMonoRepositoryChildReconciler(c reconcilers.Config) reconcilers.SubReconciler[*v1alpha1.MonoRepository] {
	return &reconcilers.ChildReconciler[*v1alpha1.MonoRepository, *apiv1beta2.GitRepository, *apiv1beta2.GitRepositoryList]{
		Name: "GitRepository",
		DesiredChild: func(ctx context.Context, parent *v1alpha1.MonoRepository) (*apiv1beta2.GitRepository, error) {
			log := util.L(ctx)

			secrets := corev1.SecretList{}
			err := c.List(ctx, &secrets, client2.InNamespace(parent.Namespace))
			if err != nil {
				return nil, errors.Wrap(err, "unable to list secrets")
			}

			serverURL, repository := ParseURLIntoServerAndPath(parent.Spec.GitRepository.URL)

			secret := FindSecret(secrets, parent.Namespace, serverURL)

			if secret == nil {
				return nil, fmt.Errorf("unable to find auth for %s", serverURL)
			}

			log.Info("constructing scm client",
				"url", serverURL,
				"kind", parent.Spec.GitRepository.Kind,
				"secret", secret.Name,
				"annotations", secret.Annotations["tekton.dev/git-0"],
				"secretsSearched", len(secrets.Items))

			client, err := factory.NewClient(parent.Spec.GitRepository.Kind,
				serverURL,
				string(secret.Data["password"]))
			if err != nil {
				return nil, errors.Wrap(err, "unable to create scmClient")
			}

			var previousCommit string
			if parent.Status.Artifact != nil {
				previousCommit = parent.Status.SHA
			}

			branch := parent.Spec.GitRepository.Branch
			subPath := parent.Spec.SubPath
			log.Info("looking for changes since",
				"repo", repository,
				"branch", branch,
				"commit", previousCommit,
				"subPath", subPath)

			// repository string, branch string, previousCommit string, subPath string
			sha, err := monorepo.DetermineClonePoint(client,
				repository,
				branch,
				previousCommit,
				subPath)
			if err != nil {
				log.Error(err, "unable to determine clone point")
				return nil, errors.Wrap(err, "unable to determine clone point")
			}

			child := &apiv1beta2.GitRepository{
				ObjectMeta: v1.ObjectMeta{
					Labels:      FilterLabelsOrAnnotations(reconcilers.MergeMaps(parent.Labels)),
					Annotations: FilterLabelsOrAnnotations(reconcilers.MergeMaps(parent.Annotations)),
					Name:        parent.Name,
					Namespace:   parent.Namespace,
				},
				Spec: apiv1beta2.GitRepositorySpec{
					URL: parent.Spec.GitRepository.URL,
					SecretRef: &meta.LocalObjectReference{
						Name: secret.Name,
					},
					Interval: v1.Duration{Duration: 0 * time.Minute},
					Reference: &apiv1beta2.GitRepositoryRef{
						Commit: sha,
					},
					Ignore:            pointer.String("\n!.git"),
					Timeout:           &v1.Duration{Duration: 1 * time.Minute},
					GitImplementation: "go-git",
				},
			}
			return child, nil
		},
		MergeBeforeUpdate: func(actual, desired *apiv1beta2.GitRepository) {
			actual.Labels = desired.Labels
			actual.Spec = desired.Spec
		},
		ReflectChildStatusOnParent: func(ctx context.Context, parent *v1alpha1.MonoRepository, child *apiv1beta2.GitRepository, err error) {
			//log := util.L(ctx)

			if err != nil {
				parent.Status.MarkFailed(ctx, err)
			}

			if child == nil {
				// if there is not child yet, there could be a failure that we've already handled?
			} else {
				// if we are ready, we should copy the childs URL & revision to the parent
				if isReady(child) {
					parent.Status.SHA = extractSha(child.Status.Artifact.Revision)
					if parent.Status.Artifact == nil {
						parent.Status.Artifact = &v1alpha1.Artifact{}
					}
					parent.Status.Artifact.URL = child.Status.Artifact.URL
					parent.Status.Artifact.Revision = child.Status.Artifact.Revision

					parent.Status.MarkReady(ctx, parent.Status.SHA)
				} else {
					// FIXME what should we do if we're not ready?
				}
			}
		},
		Sanitize: func(child *apiv1beta2.GitRepository) interface{} {
			return child.Spec
		},
	}
}

func extractSha(revision string) string {
	return strings.Split(revision, ":")[1]
}

func FindSecret(list corev1.SecretList, namespace string, serverURL string) *corev1.Secret {
	for _, secret := range list.Items {
		if secret.Type == "kubernetes.io/basic-auth" && secret.Namespace == namespace {
			// FIXME should we check for git-n here?
			val, ok := secret.Annotations["tekton.dev/git-0"]
			// FIXME should we use a string contains here?
			if ok && val == serverURL {
				return &secret
			}
		}
	}
	return nil
}

func ParseURLIntoServerAndPath(in string) (string, string) {
	u, err := url.Parse(in)
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), strings.TrimPrefix(u.Path, "/")
}

func isReady(child *apiv1beta2.GitRepository) bool {
	if len(child.Status.Conditions) > 0 {
		for _, c := range child.Status.Conditions {
			if c.Type == "Ready" {
				return c.Status == v1.ConditionTrue
			}
		}
	}
	return false
}
