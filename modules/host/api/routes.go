package api

import (
	"context"
	"net/http"

	"github.com/starius/api2"
)

// HandlerHTTPapi api interface.
type HandlerHTTPapi interface {
	TokenResources(ctx context.Context, req *TokenResourcesRequest) (*TokenResourcesResponse, error)
	ListSectorIDs(ctx context.Context, req *ListSectorIDsRequest) (*ListSectorIDsResponse, error)
	RemoveSectors(ctx context.Context, req *RemoveSectorsRequest) (*RemoveSectorsResponse, error)
	DownloadWithToken(context.Context, *DownloadWithTokenRequest) (*DownloadWithTokenResponse, error)
	UploadWithToken(context.Context, *UploadWithTokenRequest) (*UploadWithTokenResponse, error)
	AttachSectors(context.Context, *AttachSectorsRequest) (*AttachSectorsResponse, error)
}

// GetRoutes return api routes.
func GetRoutes(ol HandlerHTTPapi) []api2.Route {
	// TODO: using JSON is a temporary solution (we will work on the custom transport)
	t := &api2.JsonTransport{
		Errors: map[string]error{
			"DownloadWithTokenError": &DownloadWithTokenError{},
			"UploadWithTokenError":   &UploadWithTokenError{},
			"AttachSectorsError":     &AttachSectorsError{},
		},
	}

	return []api2.Route{
		{Method: http.MethodGet, Path: "/resources", Handler: api2.Method(&ol, "TokenResources"), Transport: t},
		{Method: http.MethodGet, Path: "/list-sector-ids", Handler: api2.Method(&ol, "ListSectorIDs"), Transport: t},
		{Method: http.MethodPost, Path: "/remove-sectors", Handler: api2.Method(&ol, "RemoveSectors"), Transport: t},
		{Method: http.MethodPost, Path: "/download", Handler: api2.Method(&ol, "DownloadWithToken"), Transport: t},
		{Method: http.MethodPost, Path: "/upload", Handler: api2.Method(&ol, "UploadWithToken"), Transport: t},
		{Method: http.MethodPost, Path: "/attach", Handler: api2.Method(&ol, "AttachSectors"), Transport: t},
	}
}
