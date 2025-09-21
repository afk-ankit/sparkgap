package main

import (
	"fmt"
	"time"

	"github.com/afk-ankit/sparkgap/breaker"
)

func accounts(s string, broke bool) (string, error) {
	if broke {
		return "", fmt.Errorf("service broke")
	}
	return fmt.Sprintf("Hi %s", s), nil
}

func main() {
	br := breaker.InitBreaker[string]("accounts", &breaker.BreakerConfig{})
	br.LogState()
	broke := false
	go func() {
		time.Sleep(time.Second * 5)
		fmt.Println("service B is down")
		broke = true
		time.Sleep(time.Second * 15)
		fmt.Println("service B is up")
		broke = false
		time.Sleep(time.Second * 5)
		fmt.Println("service B is down")
		broke = true
		time.Sleep(time.Second * 10)
		fmt.Println("service B is up")
		broke = false
	}()

	go func() {
		for {
			time.Sleep(time.Second * 5)
			br.LogState()
		}
	}()

	for range 1000 {
		time.Sleep(time.Second * 1)
		val, err := br.Execute(func() (string, error) { return accounts("ankit", broke) })
		if err != nil {
			fmt.Print(err.Error())
		}
		fmt.Println(val)
	}
}
