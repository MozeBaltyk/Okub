package environment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEndpointReachableUsesLocalFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead {
			t.Errorf("method=%s", request.Method)
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := (Local{}).EndpointReachable(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointReachableFallsBackFromHeadAndAcceptsRegistryAuth(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		if request.Method == http.MethodHead {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	if err := (Local{}).EndpointReachable(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 || methods[0] != http.MethodHead || methods[1] != http.MethodGet {
		t.Fatalf("methods=%v", methods)
	}
}
