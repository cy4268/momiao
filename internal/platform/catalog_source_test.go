package platform

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

var catalogTestNow = time.Date(2026, 9, 6, 6, 30, 0, 0, time.UTC)

func catalogTestData() string {
	return `{"schema":"momiao.native-catalog.v1","basis":"public_default_reference","billing_authority":"native_settlement","notices":["configuration_not_call_health","absence_is_not_retirement","unpublished_models_require_platform_review","extra_fees_and_integer_quota_rounding_not_included","API_Credit_is_native_USD_denominated_accounting_unit_not_currency_conversion"],"models":[{"model_id":"family/星'$(literal)","enabled_configuration":true,"native_catalog_visible":true,"endpoint_status":"configured_subset_not_health","endpoints":[{"kind":"openai","path":"/v1/chat/completions","method":"POST"}],"price":{"mode":"ratio","configured":true,"status":"reference","group_multiplier":"1","dimensions":[{"kind":"input","unit":"API_Credit_per_1M_tokens","amount":"0.00000000000000000002","source":"native_effective","condition":"uncached_plain_text_tokens","support":"not_asserted"},{"kind":"output","unit":"API_Credit_per_1M_tokens","amount":"0","source":"native_effective","condition":"plain_text_output_tokens","support":"not_asserted"},{"kind":"cache_read","unit":"API_Credit_per_1M_tokens","amount":"1","source":"native_default","condition":"only_if_native_reports_billable_cache_read_tokens","support":"not_asserted"},{"kind":"cache_write","unit":"API_Credit_per_1M_tokens","amount":"1.25","source":"native_default","condition":"only_if_native_reports_billable_generic_cache_write_tokens","support":"not_asserted"}],"unquoted_dimensions":["image","audio","tools","request_adjustments"]}}]}`
}
func catalogTestEnvelope(data string) []byte {
	return []byte(fmt.Sprintf(`{"success":true,"complete":true,"observed_at":%q,"content_hash":"sha256:%x","data":%s}`, catalogTestNow.Format(time.RFC3339Nano), sha256.Sum256([]byte(data)), data))
}
func TestNativeCatalogPreservesOpaqueIdentityExactZeroAndTinyPrice(t *testing.T) {
	got, err := ParseNativeCatalog(catalogTestEnvelope(catalogTestData()), catalogTestNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0].ModelID != "family/星'$(literal)" || *got.Models[0].Price.Dimensions[0].Amount != "0.00000000000000000002" || *got.Models[0].Price.Dimensions[1].Amount != "0" {
		t.Fatalf("identity or exact prices lost: %+v", got)
	}
}
func TestNativeCatalogRejectsEntireInvalidBatch(t *testing.T) {
	cases := map[string]func([]byte) []byte{
		"partial": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"complete":true`, `"complete":false`, 1))
		},
		"unsuccessful": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"success":true`, `"success":false`, 1))
		},
		"bad_hash":          func(b []byte) []byte { return []byte(strings.Replace(string(b), `"amount":"0"`, `"amount":"2"`, 1)) },
		"truncated":         func(b []byte) []byte { return b[:len(b)-1] },
		"too_large":         func(b []byte) []byte { return append(b, []byte(strings.Repeat(" ", 2<<20))...) },
		"trailing_document": func(b []byte) []byte { return append(b, []byte(`{}`)...) },
		"duplicate_envelope_key": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"success":true`, `"success":true,"success":true`, 1))
		},
		"missing_observed": func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"observed_at":"2026-09-06T06:30:00Z",`, "", 1))
		},
		"future_observed": func(b []byte) []byte { return []byte(strings.Replace(string(b), "06:30:00Z", "06:32:00Z", 1)) },
		"stale_observed":  func(b []byte) []byte { return []byte(strings.Replace(string(b), "06:30:00Z", "06:20:00Z", 1)) },
		"non_utc":         func(b []byte) []byte { return []byte(strings.Replace(string(b), "06:30:00Z", "14:30:00+08:00", 1)) },
	}
	dataChanges := map[string][2]string{
		"unknown_schema":         {"momiao.native-catalog.v1", "momiao.native-catalog.v2"},
		"unknown_field":          {`"models":`, `"upstream_secret":"NEVER_PERSIST","models":`},
		"case_alias":             {`"configured":`, `"Configured":`},
		"missing_boolean":        {`"native_catalog_visible":true,`, ""},
		"null_models":            {`"models":[`, `"models":null,"unused":[`},
		"bad_decimal":            {`"amount":"0"`, `"amount":"NaN"`},
		"negative_decimal":       {`"amount":"0"`, `"amount":"-1"`},
		"exponent_decimal":       {`"amount":"0"`, `"amount":"1e-20"`},
		"over_limit_decimal":     {`"amount":"0"`, `"amount":"1000000000001"`},
		"missing_price_not_zero": {`"amount":"0"`, `"amount":null`},
		"wrong_unit":             {`"API_Credit_per_1M_tokens"`, `"USD"`},
		"unknown_endpoint":       {`"/v1/chat/completions"`, `"https://private.invalid/relay"`},
		"false_health":           {`"configured_subset_not_health"`, `"healthy"`},
		"duplicate_dimension":    {`"kind":"output"`, `"kind":"input"`},
		"unknown_condition":      {`"plain_text_output_tokens"`, `"secret-expression"`},
		"unknown_notice":         {`"absence_is_not_retirement"`, `"secret_source_detail"`},
		"control_id":             {`family/星'$(literal)`, `family\nprivate`},
	}
	for name, change := range dataChanges {
		cases[name] = func(_ []byte) []byte {
			return catalogTestEnvelope(strings.Replace(catalogTestData(), change[0], change[1], 1))
		}
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseNativeCatalog(change(catalogTestEnvelope(catalogTestData())), catalogTestNow); err == nil {
				t.Fatal("invalid complete snapshot accepted")
			}
		})
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(catalogTestData()), &data); err != nil {
		t.Fatal(err)
	}
	models := data["models"].([]any)
	data["models"] = append(models, models[0])
	duplicate, _ := json.Marshal(data)
	if _, err := ParseNativeCatalog(catalogTestEnvelope(string(duplicate)), catalogTestNow); err == nil {
		t.Fatal("duplicate model ID accepted")
	}
	data["models"] = make([]any, 1001)
	for i := range data["models"].([]any) {
		data["models"].([]any)[i] = models[0]
	}
	oversized, _ := json.Marshal(data)
	if _, err := ParseNativeCatalog(catalogTestEnvelope(string(oversized)), catalogTestNow); err == nil {
		t.Fatal("over 1000 models accepted")
	}
}
func TestNativeCatalogAcceptsCompleteEmptySnapshot(t *testing.T) {
	var data map[string]any
	_ = json.Unmarshal([]byte(catalogTestData()), &data)
	data["models"] = []any{}
	raw, _ := json.Marshal(data)
	got, err := ParseNativeCatalog(catalogTestEnvelope(string(raw)), catalogTestNow)
	if err != nil || len(got.Models) != 0 {
		t.Fatalf("complete empty source is valid: %v", err)
	}
}
func TestNativeCatalogKeepsUnquotableAndConditionalDistinct(t *testing.T) {
	for _, price := range []string{
		`{"mode":"tiered_expr","configured":true,"status":"unquotable","reason":"expression_requires_usage","group_multiplier":null,"dimensions":[],"unquoted_dimensions":["image","audio","tools","request_adjustments"]}`,
		`{"mode":"per_request","configured":true,"status":"conditional","group_multiplier":"1","dimensions":[{"kind":"text_request_base","unit":"API_Credit_per_request","amount":"0","source":"native_effective","condition":"plain_text_request_without_extra_multipliers_or_tool_fees","support":"not_asserted"}],"unquoted_dimensions":["image","audio","tools","request_adjustments"]}`,
	} {
		var data map[string]any
		_ = json.Unmarshal([]byte(catalogTestData()), &data)
		var p any
		_ = json.Unmarshal([]byte(price), &p)
		data["models"].([]any)[0].(map[string]any)["price"] = p
		raw, _ := json.Marshal(data)
		got, err := ParseNativeCatalog(catalogTestEnvelope(string(raw)), catalogTestNow)
		if err != nil {
			t.Fatal(err)
		}
		if got.Models[0].Price.Status == "reference" {
			t.Fatal("unsupported ordinary rate invented")
		}
	}
}

func TestNativePersonalCatalogQuotesAreBoundToOneModelWithoutGroupConfiguration(t *testing.T) {
	var data map[string]any
	_ = json.Unmarshal([]byte(catalogTestData()), &data)
	price := data["models"].([]any)[0].(map[string]any)["price"].(map[string]any)
	price["group_multiplier"] = nil
	model := "family/星'$(literal)"
	response := map[string]any{"success": true, "schema": NativeCatalogSchema, "observed_at": catalogTestNow.Format(time.RFC3339Nano), "model_id": model, "basis": "current_user_group_reference_not_token_selection", "billing_authority": "native_settlement", "quotes": []any{map[string]any{"candidate": 1, "enabled_configuration": true, "native_catalog_visible": true, "price": price}}}
	raw, _ := json.Marshal(response)
	got, err := ParseNativePersonalCatalog(raw, model, catalogTestNow)
	if err != nil || len(got.Quotes) != 1 || got.Quotes[0].Price.GroupMultiplier != nil {
		t.Fatalf("current session quote rejected: %v", err)
	}
	if _, err = ParseNativePersonalCatalog(raw, "other/model", catalogTestNow); err == nil {
		t.Fatal("quote rebound to different model")
	}
	for name, modify := range map[string]func(){
		"internal_group":       func() { response["group"] = "private-group" },
		"selected_group_basis": func() { response["basis"] = "requested_eligible_group_reference_not_token_selection" },
		"multiplier_leak":      func() { price["group_multiplier"] = "2" },
		"no_candidates":        func() { response["quotes"] = []any{} },
		"duplicate_candidate": func() {
			response["quotes"] = []any{response["quotes"].([]any)[0], response["quotes"].([]any)[0]}
			response["basis"] = "eligible_auto_candidates_not_selected"
		},
	} {
		t.Run(name, func(t *testing.T) {
			saved, _ := json.Marshal(response)
			modify()
			bad, _ := json.Marshal(response)
			if _, err := ParseNativePersonalCatalog(bad, model, catalogTestNow); err == nil {
				t.Fatal("unsafe personal quote accepted")
			}
			response = nil // Unmarshal into a reused map retains keys absent from saved JSON.
			_ = json.Unmarshal(saved, &response)
			price = response["quotes"].([]any)[0].(map[string]any)["price"].(map[string]any)
		})
	}
	response["basis"] = "eligible_auto_candidates_not_selected"
	response["quotes"] = append(response["quotes"].([]any), map[string]any{"candidate": 2, "enabled_configuration": false, "native_catalog_visible": false, "price": nil, "reason": "model_not_enabled_in_candidate"})
	raw, _ = json.Marshal(response)
	if got, err = ParseNativePersonalCatalog(raw, model, catalogTestNow); err != nil || len(got.Quotes) != 2 || got.Quotes[1].Price != nil {
		t.Fatalf("ordered unavailable auto candidate lost: %v", err)
	}
}
