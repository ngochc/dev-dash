package secret

import "testing"

func TestMask(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "one", value: "a", want: "*"},
		{name: "eight", value: "12345678", want: "********"},
		{name: "nine", value: "123456789", want: "1234…6789"},
		{name: "token", value: "ghp_12345678", want: "ghp_…5678"},
		{name: "multibyte", value: "αβγδεζηθι", want: "αβγδ…ζηθι"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Mask(test.value); got != test.want {
				t.Errorf("Mask() = %q, want %q", got, test.want)
			}
		})
	}
}
