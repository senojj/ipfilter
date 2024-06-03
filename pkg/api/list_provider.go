package api

import (
	"firehol/pkg/badip"
	"github.com/gin-gonic/gin"
)

func ListProvider(list *badip.List) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("bad_ip_list", list)
		c.Next()
	}
}
