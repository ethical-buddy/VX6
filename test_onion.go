package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/vx6/vx6/internal/proto"
)

func main() {
	// The simulated 5-hop chain (all pointing to our local node)
	localNode := "[::1]:4242"
	hops := [5]string{localNode, localNode, localNode, localNode, localNode}
	
	header := proto.OnionHeader{
		HopCount: 0,
		Hops:     hops,
		FinalDst: "127.0.0.1:9999", // A test port we will listen on
		Payload:  []byte("HELLO FROM THE ONION CHAIN"),
	}

	fmt.Println("[TEST] Connecting to first hop...")
	conn, err := net.Dial("tcp6", localNode)
	if err != nil {
		fmt.Printf("[ERROR] Could not connect to local node: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Println("[TEST] Sending 5-hop packet...")
	_ = proto.WriteHeader(conn, proto.KindOnion)
	payload, _ := json.Marshal(header)
	_ = proto.WriteLengthPrefixed(conn, payload)

	fmt.Println("[TEST] Packet sent. Watch the node logs.")
	time.Sleep(2 * time.Second)
}
