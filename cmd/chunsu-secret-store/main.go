// chunsu-secret-store is an optional encrypted credential-helper process.
// It accepts the host protocol on stdin; secrets never appear in arguments.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"chunsu/internal/secrets"
)

const requestLimit = 16 << 10

func main() {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, requestLimit+1))
	var request secrets.HelperRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err != nil || len(data) > requestLimit || !json.Valid(data) || decoder.Decode(&request) != nil {
		fail()
	}
	response, err := secrets.EncryptedOperation(request)
	if err != nil {
		fail()
	}
	if json.NewEncoder(os.Stdout).Encode(response) != nil {
		os.Exit(1)
	}
}

func fail() {
	fmt.Fprintln(os.Stderr, "credential operation failed; inspect helper configuration without logging secret input")
	os.Exit(1)
}
