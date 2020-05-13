module ken

go 1.14

require (
	github.com/gin-gonic/gin v1.5.0
	github.com/pkg/errors v0.9.1
	github.com/scythefly/orb v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v0.0.6
	go.uber.org/zap v1.13.0
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
)

replace github.com/scythefly/orb => ../pkg/orb
