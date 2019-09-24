package test

import (
	"os"
)

var (
	doBootstrap = os.Getenv("BOOTSTRAP") == "1"
	doUpgrade   = os.Getenv("UPGRADE") == "1"
	doReboot    = os.Getenv("REBOOT") == "1"
	boot0       = os.Getenv("BOOT0")
	boot1       = os.Getenv("BOOT1")
	boot2       = os.Getenv("BOOT2")
	sshKeyFile  = os.Getenv("SSH_PRIVKEY")
	testID      = os.Getenv("TEST_ID")
	commitID    = os.Getenv("COMMIT_ID")
	externalPID = os.Getenv("EXTERNAL_PID")
	withKind    = os.Getenv("KIND") == "1"
)
