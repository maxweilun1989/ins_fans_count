package instagram_fans

import "testing"

func TestParseConfig(t *testing.T) {
	config, err := ParseConfig("../config.json")

	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
	t.Logf("Config: %v", config)
}
