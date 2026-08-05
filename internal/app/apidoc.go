package app

import (
	"context"

	"admin/internal/apidoc"
	apiapplication "admin/internal/apimetadata/application"
)

type apiDocMetadataSource struct{ core *apiapplication.Core }

func (source apiDocMetadataSource) Snapshot(ctx context.Context) ([]apidoc.Metadata, error) {
	apis, err := source.core.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]apidoc.Metadata, len(apis))
	for index, api := range apis {
		result[index] = apidoc.Metadata{
			Method: api.Method, Path: api.Path, Name: api.Name, Group: api.Group,
			Remark: api.Remark, Status: api.Status, NeedAuth: api.NeedAuth,
			NeedAudit: api.NeedAudit, PermissionCode: api.PermissionCode,
		}
	}
	return result, nil
}

var _ apidoc.MetadataSource = apiDocMetadataSource{}
