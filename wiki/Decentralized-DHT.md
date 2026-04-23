# Decentralized DHT (The Kademlia Layer)

VX6 uses a custom implementation of the Kademlia Distributed Hash Table (DHT) to ensure the network has no central point of failure.

### **How it works (XOR Metric)**
Every node in VX6 is identified by a 256-bit hash. We calculate the "distance" between any two nodes using the **XOR bitwise operation**.
*   If `Distance(A, B) < Distance(A, C)`, then Node B is "closer" to Node A.
*   Nodes keep lists of peers in "k-buckets" based on these distances.

### **The "Crawl" Search**
When you want to find a node that you don't know:
1.  Your node looks at its own k-buckets for the peers mathematically closest to the target.
2.  It asks those peers: *"Who do you know that is closer to this target?"*
3.  They return a list of even closer nodes.
4.  Your node repeats the process until it reaches the target.

### **Benefits for the Ghost Fabric**
In "Ghost Mode," the DHT is used to find **Introduction Points**. 
*   Instead of looking for a User's IP (which is hidden), you look for their "Hidden Service Descriptor."
*   The DHT tells you: *"To talk to 'surya', go to Relay-X."*
*   You never see the server's real IPv6 address, and the registry doesn't store it.

### **eBPF Acceleration**
The XOR distance calculation and bucket lookups are performed at the kernel level in the **proxy-connection** branch, ensuring that routing lookups happen in nanoseconds.
