package profile

import (
	"reflect"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"mobileconfig", `<?xml version="1.0"?>
<!DOCTYPE plist PUBLIC ...>
<plist><dict>
<key>PayloadContent</key><array></array>
</dict></plist>`, "mobileconfig"},
		{"plain plist", `<?xml version="1.0"?>
<!DOCTYPE plist PUBLIC ...>
<plist><dict>
<key>inferenceProvider</key><string>bedrock</string>
</dict></plist>`, "plist"},
		{"reg file", "Windows Registry Editor Version 5.00\r\n\r\n[HKLM\\SOFTWARE\\Policies\\Claude]", "reg"},
		{"unknown", "<html>not a plist</html>", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect([]byte(c.data)); got != c.want {
				t.Errorf("Detect = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDecodePlist_KnownKeys(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>inferenceProvider</key>
	<string>bedrock</string>
	<key>disableDeploymentModeChooser</key>
	<true/>
	<key>inferenceBedrockRegion</key>
	<string>us-west-2</string>
	<key>coworkEgressAllowedHosts</key>
	<string>["*"]</string>
</dict>
</plist>`
	p, rep, err := DecodePlist([]byte(src))
	if err != nil {
		t.Fatalf("DecodePlist: %v", err)
	}
	if len(rep.UnknownKeys) != 0 {
		t.Errorf("unexpected unknown keys: %v", rep.UnknownKeys)
	}
	if v, _ := p.Get("inferenceProvider"); v != "bedrock" {
		t.Errorf("inferenceProvider = %v", v)
	}
	if v, _ := p.Get("disableDeploymentModeChooser"); v != true {
		t.Errorf("disableDeploymentModeChooser = %v (%T)", v, v)
	}
	// stringArray round-tripped back to []string
	v, _ := p.Get("coworkEgressAllowedHosts")
	arr, ok := v.([]string)
	if !ok {
		t.Fatalf("coworkEgressAllowedHosts type = %T, want []string", v)
	}
	if !reflect.DeepEqual(arr, []string{"*"}) {
		t.Errorf("coworkEgressAllowedHosts = %v, want [*]", arr)
	}
}

func TestDecodePlist_UnknownKeyPreserved(t *testing.T) {
	src := `<?xml version="1.0"?>
<plist version="1.0">
<dict>
	<key>inferenceProvider</key>
	<string>bedrock</string>
	<key>notARealKey</key>
	<string>garbage</string>
</dict>
</plist>`
	p, rep, err := DecodePlist([]byte(src))
	if err != nil {
		t.Fatalf("DecodePlist: %v", err)
	}
	if len(rep.UnknownKeys) != 1 || rep.UnknownKeys[0].Key != "notARealKey" {
		t.Errorf("UnknownKeys = %v, want [notARealKey]", rep.UnknownKeys)
	}
	// The value is still on the profile for round-trip fidelity
	if _, ok := p.Get("notARealKey"); !ok {
		t.Error("unknown key missing from Profile.Get")
	}
	// Validate surfaces the unknown key
	err = p.Validate()
	if err == nil {
		t.Error("Validate should flag unknown key")
	}
}

func TestDecodeMobileConfig(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadType</key><string>com.anthropic.claudefordesktop</string>
			<key>PayloadIdentifier</key><string>com.yuanli.test.settings</string>
			<key>PayloadUUID</key><string>00000000-0000-0000-0000-000000000000</string>
			<key>PayloadDisplayName</key><string>yuanli-test</string>
			<key>PayloadVersion</key><integer>1</integer>
			<key>inferenceProvider</key><string>bedrock</string>
			<key>inferenceBedrockRegion</key><string>us-west-2</string>
		</dict>
	</array>
	<key>PayloadDisplayName</key><string>yuanli-test</string>
	<key>PayloadType</key><string>Configuration</string>
	<key>PayloadUUID</key><string>00000000-0000-0000-0000-000000000001</string>
	<key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>`
	p, _, err := DecodeMobileConfig([]byte(src))
	if err != nil {
		t.Fatalf("DecodeMobileConfig: %v", err)
	}
	if p.Name != "yuanli-test" {
		t.Errorf("Name = %q, want yuanli-test", p.Name)
	}
	if v, _ := p.Get("inferenceProvider"); v != "bedrock" {
		t.Errorf("inferenceProvider = %v", v)
	}
	// Payload metadata keys should NOT leak into the profile
	if _, ok := p.Get("PayloadType"); ok {
		t.Errorf("PayloadType should be stripped from decoded profile")
	}
}

func TestRoundtrip_SemanticEquivalence(t *testing.T) {
	// Encode a profile to plist, decode it back, re-encode, confirm
	// semantic equivalence (same MDM values, same order). We don't
	// compare byte-identical because there's nothing non-deterministic
	// in EncodePlist currently — but the test asserts semantic behavior
	// anyway, to catch any encoder drift.
	orig := New("rt-test")
	_ = orig.Set("inferenceProvider", "bedrock")
	_ = orig.Set("inferenceBedrockRegion", "us-west-2")
	_ = orig.Set("disableDeploymentModeChooser", true)
	_ = orig.Set("coworkEgressAllowedHosts", []string{"example.com", "*.internal"})

	encoded, err := EncodePlist(orig)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, _, err := DecodePlist(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Len() != orig.Len() {
		t.Fatalf("decoded Len = %d, orig Len = %d", decoded.Len(), orig.Len())
	}
	for _, k := range orig.Keys() {
		origV, _ := orig.Get(k)
		decV, _ := decoded.Get(k)
		if !reflect.DeepEqual(origV, decV) {
			t.Errorf("roundtrip differs at %s: orig=%v (%T) vs decoded=%v (%T)",
				k, origV, origV, decV, decV)
		}
	}
}
