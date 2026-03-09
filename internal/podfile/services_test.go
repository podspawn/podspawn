package podfile

import (
	"context"
	"fmt"
	"testing"

	"github.com/podspawn/podspawn/internal/runtime"
)

func TestStartServicesTwoServices(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	services := []ServiceConfig{
		{Name: "postgres", Image: "postgres:16"},
		{Name: "redis", Image: "redis:7"},
	}

	ids, err := StartServices(context.Background(), rt, services, "net-123", "podspawn-deploy-backend")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if len(rt.CreateCalls) != 2 {
		t.Fatalf("expected 2 create calls, got %d", len(rt.CreateCalls))
	}
	if rt.CreateCalls[0].Name != "podspawn-deploy-backend-postgres" {
		t.Errorf("service[0] name = %q", rt.CreateCalls[0].Name)
	}
	if rt.CreateCalls[1].Image != "redis:7" {
		t.Errorf("service[1] image = %q", rt.CreateCalls[1].Image)
	}
}

func TestStartServicesNetworkAlias(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	services := []ServiceConfig{
		{Name: "postgres", Image: "postgres:16"},
	}

	_, err := StartServices(context.Background(), rt, services, "net-123", "prefix")
	if err != nil {
		t.Fatal(err)
	}
	if rt.CreateCalls[0].NetworkName != "postgres" {
		t.Errorf("network alias = %q, want 'postgres'", rt.CreateCalls[0].NetworkName)
	}
	if rt.CreateCalls[0].NetworkID != "net-123" {
		t.Errorf("network ID = %q, want 'net-123'", rt.CreateCalls[0].NetworkID)
	}
}

func TestStartServicesEnvPassthrough(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	services := []ServiceConfig{
		{
			Name:  "postgres",
			Image: "postgres:16",
			Env:   map[string]string{"POSTGRES_PASSWORD": "devpass"},
		},
	}

	_, err := StartServices(context.Background(), rt, services, "net-123", "prefix")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range rt.CreateCalls[0].Env {
		if e == "POSTGRES_PASSWORD=devpass" {
			found = true
		}
	}
	if !found {
		t.Errorf("env = %v, missing POSTGRES_PASSWORD", rt.CreateCalls[0].Env)
	}
}

func TestStartServicesCreateFailure(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	services := []ServiceConfig{
		{Name: "postgres", Image: "postgres:16"},
	}

	rt.CreateErr = fmt.Errorf("image not found")
	_, err := StartServices(context.Background(), rt, services, "net-123", "prefix")
	if err == nil {
		t.Fatal("expected error on create failure")
	}
}

func TestStopServices(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.Containers["svc-1"] = true
	rt.Containers["svc-2"] = true

	StopServices(context.Background(), rt, []string{"svc-1", "svc-2"})

	if _, exists := rt.Containers["svc-1"]; exists {
		t.Error("svc-1 should be removed")
	}
	if _, exists := rt.Containers["svc-2"]; exists {
		t.Error("svc-2 should be removed")
	}
}

func TestStartServicesEmpty(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	ids, err := StartServices(context.Background(), rt, nil, "net-123", "prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty IDs, got %v", ids)
	}
	if len(rt.CreateCalls) != 0 {
		t.Error("no create calls expected for empty services")
	}
}
