//go:build !linux

package main

import "net"

// Recovery is specific to the Linux deployment. Other targets retain the
// existing refusal of any pre-existing endpoint rather than guessing ownership.
func openBotGamesListener(path string) (net.Listener, error) {
	return openListener(config{ListenSocket: path})
}
