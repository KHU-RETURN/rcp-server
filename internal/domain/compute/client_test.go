package compute

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud"
)

func TestClientCreateServerIncludesSSHOptions(t *testing.T) {
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/servers" {
			t.Fatalf("expected /servers path, got %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			"server": {
				"id": "server-1",
				"name": "test-vm",
				"status": "BUILD",
				"image": {"id": "image-1"},
				"flavor": {"id": "flavor-1"},
				"addresses": {},
				"key_name": "team-key",
				"security_groups": [{"name": "default"}, {"name": "ssh"}]
			}
		}`))
	}))
	defer ts.Close()

	sc := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{
			TokenID:    "token",
			HTTPClient: http.Client{},
		},
		Endpoint: ts.URL + "/",
	}

	server, err := createServerWithServiceClient(sc, CreateServerOpts{
		Name:           "test-vm",
		ImageRef:       "image-1",
		FlavorRef:      "flavor-1",
		KeyName:        "team-key",
		SecurityGroups: []string{"default", "ssh"},
		Networks:       []NetworkID{{UUID: "network-1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server.KeyName != "team-key" {
		t.Fatalf("expected response key_name team-key, got %q", server.KeyName)
	}

	serverBody, ok := gotBody["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level server body, got %#v", gotBody)
	}
	if serverBody["key_name"] != "team-key" {
		t.Fatalf("expected key_name in request body, got %#v", serverBody["key_name"])
	}

	networks, ok := serverBody["networks"].([]any)
	if !ok || len(networks) != 1 {
		t.Fatalf("expected one network in request body, got %#v", serverBody["networks"])
	}
	network, ok := networks[0].(map[string]any)
	if !ok || network["uuid"] != "network-1" {
		t.Fatalf("unexpected network payload: %#v", serverBody["networks"])
	}

	securityGroups, ok := serverBody["security_groups"].([]any)
	if !ok {
		t.Fatalf("expected security_groups array, got %#v", serverBody["security_groups"])
	}

	var gotGroupNames []string
	for _, rawGroup := range securityGroups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			t.Fatalf("unexpected security group payload: %#v", rawGroup)
		}
		name, _ := group["name"].(string)
		gotGroupNames = append(gotGroupNames, name)
	}

	if !reflect.DeepEqual(gotGroupNames, []string{"default", "ssh"}) {
		t.Fatalf("expected security group names [default ssh], got %#v", gotGroupNames)
	}
}

func TestClientCreateServerAlwaysUsesLocalDisk(t *testing.T) {
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"server": {"id": "server-1", "status": "BUILD"}}`))
	}))
	defer ts.Close()

	sc := &gophercloud.ServiceClient{
		ProviderClient: &gophercloud.ProviderClient{HTTPClient: http.Client{}},
		Endpoint:       ts.URL + "/",
	}

	// RootVolumeSizeGB를 10으로 줘도 로컬 부팅이어야 함
	_, _ = createServerWithServiceClient(sc, CreateServerOpts{
		Name:             "tiny-vm",
		ImageRef:         "image-1",
		RootVolumeSizeGB: 10,
	})

	serverBody := gotBody["server"].(map[string]any)
	
	// 1. imageRef가 반드시 있어야 함 (로컬 부팅의 증거)
	if _, ok := serverBody["imageRef"]; !ok {
		t.Fatal("expected imageRef to be present for local disk boot")
	}
	// 2. block_device_mapping_v2가 없어야 함 (볼륨 부팅이 아니라는 증거)
	if _, ok := serverBody["block_device_mapping_v2"]; ok {
		t.Fatal("expected no block_device_mapping_v2 for local disk boot")
	}
}
