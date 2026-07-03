package domain

import "gin-template/internal/domain/demo"

// ServiceHub 服务聚合中心，集中管理所有业务服务
// 每新增一个业务模块，在此添加对应的 Service 字段
type ServiceHub struct {
	DemoService *demo.DemoService
}

func NewServiceHub(demoService *demo.DemoService) *ServiceHub {
	return &ServiceHub{
		DemoService: demoService,
	}
}
