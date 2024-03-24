# firefox_container

A Windows driver for automated authentications using Firefox Portable.

## Installation

In your Go project, run

```shell
go get -u github.com/enzo-santos/firefox_container
```

and use it in your code:

```go
package main

import (
	"github.com/enzo-santos/firefox_container"
)

func main() {
	firefox := firefox_container.FirefoxPortable{
		Path:           "C:/Users/User/FirefoxPortable",
		ExecutableName: "FirefoxPortable.exe",
	}
}
```

## Usage

TBA
