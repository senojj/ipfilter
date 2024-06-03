package api

import (
	"github.com/gin-gonic/gin"
	"ipfilter/pkg/iplist"
)

func ListProvider(list *iplist.List) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("bad_ip_list", list)
		c.Next()
	}
}
