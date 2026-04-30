package cmdlifecycle

import (
	"testing"

	"go.uber.org/fx"
)

func TestModule_ProvidesService(t *testing.T) {
	var svc *Service
	app := fx.New(
		fx.Provide(func() Repository { return nullRepo{} }),
		Module,
		fx.Populate(&svc),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New: %v", err)
	}
	if svc == nil {
		t.Fatal("Service was not provided")
	}
}
