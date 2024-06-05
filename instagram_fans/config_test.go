package instagram_fans

import "testing"

func TestParseConfig(t *testing.T) {
	config := ParseConfig("../config.json")
	t.Logf("Config: %v", config)
}
