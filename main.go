package main

import (
	"context"

	"admin/internal/app"
)

func main() {
	ctx := context.Background()
	application, err := app.BuildFromPath(ctx, "config.yaml", app.Options{})
	if err != nil {
		panic("build application: " + err.Error())
	}
	defer func() {
		if err := application.Close(); err != nil {
			panic("close application: " + err.Error())
		}
	}()
	if err := application.Run(ctx); err != nil {
		panic("run application: " + err.Error())
	}
}
