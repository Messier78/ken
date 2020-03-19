package monitor

import (
	"github.com/gin-gonic/gin"

	"ken/monitor/rtmp"
)

// Start
func Start() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	rtmpGroup := r.Group("/rtmp")
	{
		rtmpGroup.GET("/monitor", rtmp.HandleMonitor)
	}

	r.Run("localhost:9099")
}
