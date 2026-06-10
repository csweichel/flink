package cmd

import (
	"context"
	"os"
	"os/signal"

	serverapp "github.com/csweichel/flink/server/app"
)

func runServe(configPath string, options Options) error {
	cfg, err := loadServerConfig(configPath)
	if err != nil {
		return err
	}
	if err := bootstrapConfiguredTenants(cfg); err != nil {
		return err
	}

	app := serverapp.New(serverapp.Config{
		DataDir:            cfg.DataDir,
		StorageDriver:      cfg.StorageDriver,
		BaseHost:           cfg.BaseHost,
		AutoApproveTenants: cfg.AutoApproveTenants,
		AI:                 cfg.AI,
	})
	if err := app.Init(); err != nil {
		return err
	}
	defer app.Close()
	signalContext := options.SignalContext
	if signalContext == nil {
		signalContext = defaultSignalContext
	}
	ctx, stop := signalContext()
	defer stop()
	return serverapp.ListenAndServe(ctx, cfg.Addr, app)
}

func defaultSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
