package main

import (
	"runtime"

	"api-automation/internal/app"
)

func main() {
	runtime.LockOSThread() // Win32 GUI must live on one OS thread.
	app.Run()
}
