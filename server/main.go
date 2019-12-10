package main

import (
	"fmt"
	"github.com/syleron/426c/common/security"
)

var (
	port = 9000
)

func main() {
	fmt.Println(`
  ____ ___  ____    
 / / /|_  |/ __/____
/_  _/ __// _ \/ __/
 /_//____/\___/\__/
        www.426c.net
`)
	// Generate new RSA keys
	if err := security.GenerateKeys("127.0.0.1"); err != nil {
		panic(err)
	}
	// Create new instance of server
	server := setupServer(fmt.Sprintf(":%v", port))
	defer server.shutdown()
	// Handle incoming connections
	server.connectionHandler()
}

