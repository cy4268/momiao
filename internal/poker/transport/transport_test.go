package transport

import (
	"encoding/json"
	"testing"
)

func TestConstructorRequiresVerifiedAuthority(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("missing authorities accepted")
	}
}

func TestStrictClientEnvelope(t *testing.T) {
	good := `{"type":"ping","request_id":"request-00000001","table_id":"11111111-1111-4111-8111-111111111111","hand_id":null,"action_id":null,"expected_table_version":9007199254740991,"expected_hand_version":0,"control_epoch":0,"payload":{}}`
	if _, err := decodeClient([]byte(good)); err != nil {
		t.Fatal(err)
	}
	bad := []string{
		`{"type":"ping","type":"auth.connect"}`,
		`{"type":"ping","USER_ID":9}`,
		`{"type":"ping","expected_table_version":9007199254740992}`,
		`{"type":"ping","expected_table_version":"1"}`,
		`{"type":"ping","expected_table_version":1.1}`,
		`{"type":"ping","expected_table_version":-1}`,
		`{"type":"ping","payload":{"x":1,"x":2}}`,
		`null`, good + `{}`,
	}
	for _, body := range bad {
		if _, err := decodeClient([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	var n SafeNumber
	if err := json.Unmarshal([]byte(`null`), &n); err == nil {
		t.Fatal("null counter accepted")
	}
}

func TestStrictJSONRejectsNullScalar(t *testing.T) {
	var v struct {
		Allow bool `json:"allow"`
	}
	if err := strictJSON([]byte(`{"allow":null}`), &v, "allow"); err == nil {
		t.Fatal("null silently became false")
	}
}
