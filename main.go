package main

import (
	"log"

	"github.com/morganhein/garlic"
	"github.com/morganhein/yourbase-challenge/tracker"
)

func main() {
	//I have only been able to get this to work when the initial Dial is within
	//main
	garConn, err := garlic.DialPCN()

	if err != nil {
		log.Fatalf("error initiliazing garlic: %s", err)
	}

	log.Println("Garlic connection created.")
	// tracker.Launch(&garConn, "docker", "build", ".", "--no-cache")
	tracker.Launch(&garConn, "docker", "build", "-f", "Dockerfile.small", ".", "--no-cache")
}
