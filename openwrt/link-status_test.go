package main

import "testing"

func TestHotspotActiveNotHistorical(t *testing.T) {
	head := "Dump of WifiActiveModeWarden\nNumActiveModeManagers: 1\n"
	old := "Dump of ActiveModeWarden.Graveyard\nDump of SoftApManager id=1\ncurrent StateMachine mode: StartedState\nmIfaceIsUp: true\n"
	cases := []struct{ raw, want string }{{head + old, "disabled"}, {head + "Dump of SoftApManager id=2\ncurrent StateMachine mode: StartedState\nmIfaceIsUp: true\n" + old, "enabled"}, {head + "Dump of SoftApManager id=2\ncurrent StateMachine mode: IdleState\n" + old, "transitioning"}, {"permission denied", "unknown"}}
	for _, c := range cases {
		if got := parseHotspotStatus(c.raw); got != c.want {
			t.Errorf("got %s want %s", got, c.want)
		}
	}
}
