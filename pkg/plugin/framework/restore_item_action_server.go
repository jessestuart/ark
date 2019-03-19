/*
Copyright 2017, 2019 the Velero contributors.

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

package framework

import (
	"encoding/json"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	api "github.com/heptio/velero/pkg/apis/velero/v1"
	proto "github.com/heptio/velero/pkg/plugin/generated"
	"github.com/heptio/velero/pkg/plugin/velero"
)

// RestoreItemActionGRPCServer implements the proto-generated RestoreItemActionServer interface, and accepts
// gRPC calls and forwards them to an implementation of the pluggable interface.
type RestoreItemActionGRPCServer struct {
	mux *serverMux
}

func (s *RestoreItemActionGRPCServer) getImpl(name string) (velero.RestoreItemAction, error) {
	impl, err := s.mux.getHandler(name)
	if err != nil {
		return nil, err
	}

	itemAction, ok := impl.(velero.RestoreItemAction)
	if !ok {
		return nil, errors.Errorf("%T is not a restore item action", impl)
	}

	return itemAction, nil
}

func (s *RestoreItemActionGRPCServer) AppliesTo(ctx context.Context, req *proto.AppliesToRequest) (response *proto.AppliesToResponse, err error) {
	defer func() {
		if recoveredErr := handlePanic(recover()); recoveredErr != nil {
			err = recoveredErr
		}
	}()

	impl, err := s.getImpl(req.Plugin)
	if err != nil {
		return nil, newGRPCError(err)
	}

	appliesTo, err := impl.AppliesTo()
	if err != nil {
		return nil, newGRPCError(err)
	}

	return &proto.AppliesToResponse{
		IncludedNamespaces: appliesTo.IncludedNamespaces,
		ExcludedNamespaces: appliesTo.ExcludedNamespaces,
		IncludedResources:  appliesTo.IncludedResources,
		ExcludedResources:  appliesTo.ExcludedResources,
		Selector:           appliesTo.LabelSelector,
	}, nil
}

func (s *RestoreItemActionGRPCServer) Execute(ctx context.Context, req *proto.RestoreExecuteRequest) (response *proto.RestoreExecuteResponse, err error) {
	defer func() {
		if recoveredErr := handlePanic(recover()); recoveredErr != nil {
			err = recoveredErr
		}
	}()

	impl, err := s.getImpl(req.Plugin)
	if err != nil {
		return nil, newGRPCError(err)
	}

	var (
		item           unstructured.Unstructured
		itemFromBackup unstructured.Unstructured
		restoreObj     api.Restore
	)

	if err := json.Unmarshal(req.Item, &item); err != nil {
		return nil, newGRPCError(errors.WithStack(err))
	}

	if err := json.Unmarshal(req.ItemFromBackup, &itemFromBackup); err != nil {
		return nil, newGRPCError(errors.WithStack(err))
	}

	if err := json.Unmarshal(req.Restore, &restoreObj); err != nil {
		return nil, newGRPCError(errors.WithStack(err))
	}

	executeOutput, err := impl.Execute(&velero.RestoreItemActionExecuteInput{
		Item:           &item,
		ItemFromBackup: &itemFromBackup,
		Restore:        &restoreObj,
	})
	if err != nil {
		return nil, newGRPCError(err)
	}

	updatedItem, err := json.Marshal(executeOutput.UpdatedItem)
	if err != nil {
		return nil, newGRPCError(errors.WithStack(err))
	}

	var warnMessage string
	if executeOutput.Warning != nil {
		warnMessage = executeOutput.Warning.Error()
	}

	return &proto.RestoreExecuteResponse{
		Item:    updatedItem,
		Warning: warnMessage,
	}, nil
}
