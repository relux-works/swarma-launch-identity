package identity

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Pins literal encoding and digest independently of the implementation;
// repeated calls prove fixture determinism, not universal collision freedom.
func TestPublicVectors(t *testing.T) {
	data, err := os.ReadFile("testdata/public-encoding-vectors-r0.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Host    string `json:"host_id"`
		Project string `json:"project_id"`
		Profile string `json:"profile_id"`
		Encoded string `json:"encoded_hex"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 4 {
		t.Fatal("expected four public vectors")
	}
	for _, v := range vectors {
		ids := IDs{v.Host, v.Project, v.Profile}
		want, err := hex.DecodeString(v.Encoded)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Encode(ids)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("literal encoding mismatch: %x, %v", got, err)
		}
		for range 2 {
			name, err := Derive(ids)
			if err != nil || name != v.Name {
				t.Fatalf("literal name mismatch: %q, %v", name, err)
			}
		}
	}
}

// Pins individual ID changes, Unicode forms, whitespace and case to external
// literal SHA-256 values. These fixtures disprove folding, not all collisions.
func TestExactInputs(t *testing.T) {
	cases := []struct {
		ids  IDs
		want string
	}{
		{IDs{"h", "p", "r"}, "dabc9bcf3b62c00fb8cefde216710dbae06b8e96bef04b8cdde47cf3f4e746f3"},
		{IDs{"H", "p", "r"}, "2223cb7ae0312ec1f3b8ad7f5dd63e9e43215083f929a04e7fa6acdfb66fc8c5"},
		{IDs{"h", "P", "r"}, "5748648a86a7ae264d295abe23dc809b999f860893b0a6fb301f1e419f018885"},
		{IDs{"h", "p", "R"}, "c98452e1d19ce8798a82146b1976200d481bc4d420ea7a876db4627b8d406955"},
		{IDs{" h", "p", "r"}, "f6524dc7d8533094abc8436076e1135bec59bd6b1b102899b1f052def8938e07"},
		{IDs{"é", "p", "r"}, "11ae2f39033eaca9f1376807e9490f7b1354e8316cccd181f71cbd22fd2aecbe"},
		{IDs{"e\u0301", "p", "r"}, "d61941dfef29acfc5e0fecadfcf8cdba3e94e08ab39ddb76f3eac86c3cabdc55"},
	}
	for _, c := range cases {
		got, err := Derive(c.ids)
		if err != nil || got != "syn1:"+c.want {
			t.Fatalf("exact-input name mismatch: %q %v", got, err)
		}
	}
}

// Every field uses the same byte/control admission rules. Valid format
// characters (not Unicode controls) and opaque punctuation remain admitted.
func TestRepresentation(t *testing.T) {
	valid := []string{"x", " ", "--opaque", "a/b", "\u200d", strings.Repeat("x", 256), strings.Repeat("é", 128), strings.Repeat("é", 127) + "ab"}
	invalid := []string{"", "\xff", "\x00", "\x1f", "\x7f", "\u0085", strings.Repeat("x", 257), strings.Repeat("é", 128) + "x", strings.Repeat("é", 129)}
	for field := 0; field < 3; field++ {
		for _, group := range []struct {
			values []string
			pass   bool
		}{{valid, true}, {invalid, false}} {
			for _, value := range group.values {
				values := []string{"h", "p", "r"}
				values[field] = value
				ids := IDs{values[0], values[1], values[2]}
				encoded, err := Encode(ids)
				name, nameErr := Derive(ids)
				if group.pass {
					if len(value) == 256 && err == nil {
						offset := len("swarma.synthetic-identity/v1\x00")
						for prior := 0; prior < field; prior++ {
							offset += 4 + len(values[prior])
						}
						if len(encoded) < offset+4+256 || !bytes.Equal(encoded[offset:offset+4], []byte{0, 0, 1, 0}) || !bytes.Equal(encoded[offset+4:offset+4+256], []byte(value)) {
							t.Fatal("literal 256-byte prefix or payload mismatch")
						}
					}
					if err != nil || nameErr != nil || len(name) != 69 || len(encoded) != len(domain)+12+len(ids.Host)+len(ids.Project)+len(ids.Profile) {
						t.Fatal("valid representation refused or truncated")
					}
				} else if !errors.Is(err, ErrID) || !errors.Is(nameErr, ErrID) || encoded != nil || name != "" {
					t.Fatal("invalid representation admitted")
				}
			}
		}
	}
}
