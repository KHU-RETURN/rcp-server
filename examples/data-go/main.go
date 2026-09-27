package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	input := bufio.NewScanner(os.Stdin)
	if !input.Scan() {
		panic("missing event")
	}
	fmt.Println(`{"$rcp":"db","op":"put","collection":"visits","key":"count","value":1}`)
	if !input.Scan() || !strings.Contains(input.Text(), `"ok":true`) {
		panic("put failed")
	}
	fmt.Println(`{"$rcp":"db","op":"get","collection":"visits","key":"count"}`)
	if !input.Scan() || !strings.Contains(input.Text(), `"value":1`) {
		panic("get failed")
	}
	fmt.Println(`{"statusCode":200,"headers":{"content-type":"application/json"},"body":"{\"count\":1}"}`)
}
