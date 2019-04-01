package main

import (
	"fmt"

	"github.com/fearful-symmetry/garlic"

	"github.com/prometheus/common/log"
)

func main() {
	cn, err := garlic.DialPCN()
	if err != nil {
		log.Fatalf("%s", err)
	}

	//Read in events
	for {
		data, err := cn.ReadPCN()

		if err != nil {
			log.Errorf("Read fail: %s", err)
		}
		fmt.Printf("%#v\n", data)
	}
}
