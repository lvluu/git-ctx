// Package ui handles interactive user prompts.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lvluu/git-ctx/internal/profile"
)

// PromptProfileName prompts the user for a profile name.
func PromptProfileName(_ []string) (string, error) {
	fmt.Print("Enter profile name: ")
	reader := bufio.NewReader(os.Stdin)
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	return name, nil
}

// PromptProfileDetails prompts for profile name and email.
func PromptProfileDetails(existing *profile.Profile) profile.Profile {
	reader := bufio.NewReader(os.Stdin)
	p := profile.Profile{}

	if existing != nil && existing.Name != "" {
		fmt.Printf("\nEnter name [current: %s, press Enter to keep]: ", existing.Name)
	} else {
		fmt.Print("Enter name: ")
	}
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" && existing != nil {
		p.Name = existing.Name
	} else {
		p.Name = name
	}

	if existing != nil && existing.Email != "" {
		fmt.Printf("Enter email [current: %s, press Enter to keep]: ", existing.Email)
	} else {
		fmt.Print("Enter email: ")
	}
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" && existing != nil {
		p.Email = existing.Email
	} else {
		p.Email = email
	}

	fmt.Print("Enter signing key (optional, press Enter to skip): ")
	signingKey, _ := reader.ReadString('\n')
	signingKey = strings.TrimSpace(signingKey)
	if signingKey != "" {
		p.Signing.Key = signingKey
	} else if existing != nil {
		p.Signing.Key = existing.Signing.Key
	}

	return p
}
