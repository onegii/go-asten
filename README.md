[![Go Reference](https://pkg.go.dev/badge/github.com/onegii/go-asten/asten.svg)](https://pkg.go.dev/github.com/onegii/go-asten/asten)
[![Go Report Card](https://goreportcard.com/badge/github.com/onegii/go-asten)](https://goreportcard.com/report/github.com/onegii/go-asten)

# Go-asten
Go-asten provides functionalities for runtime performance evaluation.
  
The project is still in developement, feedback is welcome.

# Overview
Statistics are generated using timers and organized in Groups.
Each group contains one or more Profiles.
Each Profile may be composite, i.e., a collection of sub-profiles.
An example structure may be:

```
 Group 1
  ├ Profile 1.1
  └ Profile 1.2
    ├ Sub-Profile 1.2.1
    └ Sub-Profile 1.2.2

 Group 2
  └ Profile 2.1
    └ Sub-Profile 2.1.1
      └ Sub-Profile 2.1.1.1
```

# Installing
Use `go get` to install the latest version.

```
go get -u github.com/onegii/go-asten
```
Then, simply import Asten in your application:
```go
import "github.com/onegii/go-asten/asten"

```

# Usage
Statistics are presented in a tabular manner. For example:

```go
import (
	"math/big"
	"time"

	"github.com/onegii/go-asten/asten"
)

func main() {
	// divProfile will measure the time it takes to check whether a number is
	// odd, even and divisible by 4
	divProfile := asten.Profile("divisions").MakeComposite()
	divProfile.Profile("even").MakeComposite()

	// primeProfile will measure the time it takes to check if a number is prime
	primeProfile := asten.Profile("primes").MakeComposite()

	for i := int64(0); i < 1000; i++ {
		t := primeProfile.StartTimer()
		if big.NewInt(i).ProbablyPrime(0) {
			t.StopAs("prime")
		} else {
			t.StopAs("not_prime")
		}

		t = divProfile.StartTimer()
		if i%4 == 0 {
			t.StopAs("even", "div_by_4")
			continue
		}
		if i%2 == 0 {
			t.StopAs("even")
			continue
		}
		t.StopAs("odd")
	}

	// mock group
	t := asten.Group("go-asten").Profile("asten").StartTimer()
	time.Sleep(time.Second)
	t.Stop()

	asten.PrintGroups()
}

```
  
Outputs:  

![readme_example_output](https://github.com/onegii/go-asten/assets/111180807/463a31a3-bd7f-4ab5-a1c7-6ee09c97bbaf)
