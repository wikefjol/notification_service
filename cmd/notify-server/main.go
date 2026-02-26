// Command notify-server is the notification server daemon.
package main

import (
	"os"

	"github.com/wikefjol/notification_service/server"
)

func main() {
	os.Exit(server.Run())
}
