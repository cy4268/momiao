package main

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/cy4268/momiao/internal/identity"
)

func readDiscordConfig(path, origin string) (identity.Discord, error) {
	var discord identity.Discord
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &discord) != nil || discord.ClientID == "" || discord.ClientSecret == "" {
		return identity.Discord{}, errors.New("MOMIAO_DISCORD_CONFIG_FILE requires private JSON with client_id and client_secret")
	}
	discord.RedirectURI = origin + "/oauth/discord"
	return discord, nil
}
