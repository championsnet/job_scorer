package multitenant

import (
	"context"
	"fmt"
	"net/http"
)

func Run(ctx context.Context) error {
	runtimeCfg, err := LoadRuntimeConfig()
	if err != nil {
		return err
	}

	switch runtimeCfg.Mode {
	case "web":
		server, err := NewServer(ctx, runtimeCfg)
		if err != nil {
			return err
		}
		defer server.Close()
		if err := server.ConfigureEnqueuer(ctx); err != nil {
			return err
		}
		return http.ListenAndServe(":"+runtimeCfg.Port, server.WebHandler())
	case "worker":
		server, err := NewServer(ctx, runtimeCfg)
		if err != nil {
			return err
		}
		defer server.Close()
		return http.ListenAndServe(":"+runtimeCfg.Port, server.WorkerHandler())
	case "import":
		return RunImport(ctx, runtimeCfg)
	default:
		return fmt.Errorf("unsupported APP_MODE: %s", runtimeCfg.Mode)
	}
}
