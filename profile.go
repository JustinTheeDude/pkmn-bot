package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Profile holds checkout details. Stored ONLY in local profile.json (0600).
// Never logged, never sent anywhere except the retailer checkout you trigger.
type Profile struct {
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Address1   string `json:"address1"`
	Address2   string `json:"address2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	Zip        string `json:"zip"`
	Country    string `json:"country"`
	CardName   string `json:"card_name"`
	CardNumber string `json:"card_number"`
	ExpMonth   string `json:"exp_month"`
	ExpYear    string `json:"exp_year"`
	CVV        string `json:"cvv"`
}

func defaultProfile() Profile {
	return Profile{Country: "US"}
}

func loadProfile(path string) (Profile, error) {
	p := defaultProfile()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		return defaultProfile(), fmt.Errorf("parse %s: %w", path, err)
	}
	return p, nil
}

// saveProfile writes with 0600 perms (owner read/write only).
func saveProfile(path string, p Profile) error {
	if err := validateProfile(p); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func validateProfile(p Profile) error {
	if p.Email != "" && !strings.Contains(p.Email, "@") {
		return fmt.Errorf("invalid email")
	}
	digits := digitOnly(p.CardNumber)
	if p.CardNumber != "" && len(digits) < 13 {
		return fmt.Errorf("card number looks too short")
	}
	if p.ExpMonth != "" {
		m := atoiSafe(p.ExpMonth)
		if m < 1 || m > 12 {
			return fmt.Errorf("exp month must be 01-12")
		}
	}
	if p.CVV != "" && (len(p.CVV) < 3 || len(p.CVV) > 4) {
		return fmt.Errorf("cvv must be 3-4 digits")
	}
	return nil
}

// maskedProfile is what the UI GET endpoint returns — never full PAN/CVV.
func maskedProfile(p Profile) map[string]string {
	last4 := ""
	if d := digitOnly(p.CardNumber); len(d) >= 4 {
		last4 = d[len(d)-4:]
	}
	card := ""
	if last4 != "" {
		card = "•••• •••• •••• " + last4
	}
	cvv := ""
	if p.CVV != "" {
		cvv = "***"
	}
	return map[string]string{
		"first_name": p.FirstName, "last_name": p.LastName,
		"email": p.Email, "phone": p.Phone,
		"address1": p.Address1, "address2": p.Address2,
		"city": p.City, "state": p.State, "zip": p.Zip, "country": p.Country,
		"card_name": p.CardName, "card_number_masked": card,
		"card_last4": last4, "has_card": boolStr(p.CardNumber != ""),
		"exp_month": p.ExpMonth, "exp_year": p.ExpYear, "cvv_masked": cvv,
		"has_cvv": boolStr(p.CVV != ""),
	}
}

func digitOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
