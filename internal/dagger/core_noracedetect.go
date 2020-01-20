// +build !race

package dagger

func init() {
	CheckGoroutineShutdown = true
}
