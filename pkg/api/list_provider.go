package api

import (
	"firehol/pkg/badip"
	"firehol/pkg/config"

	"github.com/gin-gonic/gin"
)

func ListProvider() gin.HandlerFunc {
	settings, err := config.Load("./config.json")
	if err != nil {
		panic(err)
	}

	loader := badip.NewGitHubLoader(settings.ArchiveURL, settings.FileSuffixList)
	list := badip.NewList(1_000_000)
	found, err := loader.Load(&list)
	if err != nil {
		println(err.Error())
	}

	if list.Len() != found {
		panic("found bad addresses not equal to stored")
	}

	return func(c *gin.Context) {
		c.Set("bad_ip_list", &list)
		c.Next()
	}
}
