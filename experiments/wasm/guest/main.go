// WASM guest: tries to read a mounted file and a host file it should not see.
package main

import (
	"fmt"
	"os"
)

func main() {
	b, err := os.ReadFile("/work/wasmdemo.txt")
	fmt.Printf("/work/wasmdemo.txt: %q err=%v\n", string(b), err)
	_, err = os.ReadFile("/etc/passwd")
	fmt.Println("/etc/passwd:", err, "(expected: an error, the host FS is invisible)")
}
