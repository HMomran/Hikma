package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// VAPIDKeys stores the public and private VAPID keys
type VAPIDKeys struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

func main() {
	log.Println("🔑 Generating VAPID keys for Daily Hikmah...")

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate VAPID keys: %v", err)
	}

	keys := VAPIDKeys{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}

	// Save to JSON file
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		log.Fatalf("❌ Failed to marshal keys: %v", err)
	}

	filename := "vapid_keys.json"
	if err := os.WriteFile(filename, data, 0600); err != nil {
		log.Fatalf("❌ Failed to save keys to file: %v", err)
	}

	log.Printf("✅ VAPID keys generated and saved to %s", filename)
	log.Println()
	log.Println("📋 Your VAPID Keys:")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Public Key:  %s\n", publicKey)
	fmt.Printf("Private Key: %s\n", privateKey)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()
	log.Println("📝 Next Steps:")
	log.Println("1. Copy the keys above")
	log.Println("2. Set them as environment variables on Render:")
	log.Println("   VAPID_PUBLIC_KEY=<public key>")
	log.Println("   VAPID_PRIVATE_KEY=<private key>")
	log.Println()
	log.Println("⚠️  Keep these keys secret! Don't commit them to git.")
}
