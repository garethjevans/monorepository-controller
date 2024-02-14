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
	"io"
	"os"
	"path/filepath"
	"strings"

	apiv1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/garethjevans/monorepository-controller/api/v1alpha1"
	"github.com/garethjevans/monorepository-controller/internal/util"
	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	"golang.org/x/mod/sumdb/dirhash"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=source.garethjevans.org,resources=monorepositories/finalizers,verbs=update
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=patch;create;update

func NewMonoRepositoryReconciler(c reconcilers.Config) *reconcilers.ResourceReconciler[*v1alpha1.MonoRepository] {
	return &reconcilers.ResourceReconciler[*v1alpha1.MonoRepository]{
		Name: "MonoRepository",
		Reconciler: reconcilers.Sequence[*v1alpha1.MonoRepository]{
			NewResourceValidator(c),
		},
		Config: c,
	}
}

func NewResourceValidator(c reconcilers.Config) reconcilers.SubReconciler[*v1alpha1.MonoRepository] {
	return &reconcilers.ChildReconciler[*v1alpha1.MonoRepository, *apiv1beta2.GitRepository, *apiv1beta2.GitRepositoryList]{
		Name: "GitRepository",
		DesiredChild: func(ctx context.Context, parent *v1alpha1.MonoRepository) (*apiv1beta2.GitRepository, error) {
			child := &apiv1beta2.GitRepository{
				ObjectMeta: v1.ObjectMeta{
					Labels:      FilterLabelsOrAnnotations(reconcilers.MergeMaps(parent.Labels)),
					Annotations: FilterLabelsOrAnnotations(reconcilers.MergeMaps(parent.Annotations)),
					Name:        parent.Name,
					Namespace:   parent.Namespace,
				},
				Spec: parent.Spec.GitRepository,
			}

			return child, nil
		},
		MergeBeforeUpdate: func(actual, desired *apiv1beta2.GitRepository) {
			actual.Labels = desired.Labels
			actual.Spec = desired.Spec
		},
		ReflectChildStatusOnParent: func(ctx context.Context, parent *v1alpha1.MonoRepository, child *apiv1beta2.GitRepository, err error) {
			log := util.L(ctx)
			if parent == nil || child == nil {
				return
			}
			if !isReady(child) {
				return
			}
			if !hasArtifactURL(child) {
				log.Info("no artifactURL present")
				return
			}

			tempDir, err := os.MkdirTemp("", "tmp")
			if err != nil {
				parent.Status.MarkFailed(ctx, err)
				return
			}
			// cleanup on exit
			defer os.RemoveAll(tempDir)

			log.Info("created temp dir", "dir", tempDir)

			// download the filter and copy from/to path
			tarGzLocation := filepath.Join(tempDir, fmt.Sprintf("%s.tar.gz", child.Name))
			if err := util.DownloadFile(tarGzLocation, child.Status.Artifact.URL); err != nil {
				parent.Status.MarkFailed(ctx, err)
				return
			}

			// extract tar.gz to temp location
			tarGzExtractedLocation := filepath.Join(tempDir, fmt.Sprintf("%s-extracted", child.Name))
			if err := util.ExtractTarGz(tarGzLocation, tarGzExtractedLocation); err != nil {
				parent.Status.MarkFailed(ctx, err)
				return
			}

			files, err := util.ListFiles(tarGzExtractedLocation)
			if err != nil {
				parent.Status.MarkFailed(ctx, err)
				return
			}

			log.Info("Full file list", "files", files)
			filteredFiles := util.FilterFileList(files, parent.Spec.Include)
			log.Info("Using files for checksum calculation", "files", filteredFiles)
			parent.Status.ObservedFileList = strings.Join(filteredFiles, "\n")

			hash, err := dirhash.Hash1(filteredFiles, func(name string) (io.ReadCloser, error) {
				return os.Open(filepath.Join(tarGzExtractedLocation, name))
			})
			if err != nil {
				parent.Status.MarkFailed(ctx, err)
				return
			}

			log.Info("Calculated checksum", "checksum", hash)

			if parent.Status.Artifact != nil && parent.Status.Artifact.Checksum == hash {
				log.Info("Source hasn't changed, there is nothing to update")
				parent.Status.MarkReady(ctx, hash)
				return
			}

			old := "<NA>"
			if parent.Status.Artifact != nil {
				old = parent.Status.Artifact.Checksum
			}

			log.Info("Source has changed! updating status with new checksum", "checksum", hash, "old", old)
			parent.Status.Artifact = &v1alpha1.Artifact{
				Path:           child.Status.Artifact.Path,
				URL:            child.Status.Artifact.URL,
				Revision:       child.Status.Artifact.Revision,
				Checksum:       hash,
				Digest:         child.Status.Artifact.Digest,
				LastUpdateTime: child.Status.Artifact.LastUpdateTime,
				Size:           child.Status.Artifact.Size,
				Metadata:       child.Status.Artifact.Metadata,
			}
			parent.Status.URL = child.Status.Artifact.URL
			parent.Status.MarkReady(ctx, hash)
		},
		Sanitize: func(child *apiv1beta2.GitRepository) any {
			return child.Spec
		},
	}
}

func hasArtifactURL(child *apiv1beta2.GitRepository) bool {
	if child.Status.Artifact == nil {
		return false
	}
	if len(child.Status.Artifact.URL) == 0 {
		return false
	}
	return true
}

func isReady(child *apiv1beta2.GitRepository) bool {
	if child == nil {
		return false
	}

	for _, c := range child.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == v1.ConditionTrue
		}
	}
	return false
}
