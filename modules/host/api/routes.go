package api

import (
	"context"
	"net/http"

	"github.com/starius/api2"
)

// HandlerHTTPapi api interface
type HandlerHTTPapi interface {
	DownloadWithToken(context.Context, *DownloadWithTokenRequest) (*DownloadWithTokenResponse, error)
}

// GetRoutes return api routes
func GetRoutes(ol HandlerHTTPapi) []api2.Route {
	// TODO: using JSON is a temporary solution (we will work on the custom transport)
	t := &api2.JsonTransport{
		Errors: map[string]error{"DownloadWithTokenError": DownloadWithTokenError{}},
	}

	return []api2.Route{
		{Method: http.MethodPost, Path: "/download", Handler: api2.Method(&ol, "DownloadWithToken"), Transport: t},
	}
}
