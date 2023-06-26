// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build (darwin || linux || openbsd || netbsd) && !nomeminfo
// +build darwin linux openbsd netbsd
// +build !nomeminfo

//定义了一个名为collector的Go包
package collector

//导入所需的外部包或库
import (
	"fmt"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

//定义了一个名为memInfoSubsystem的常量，值为"memory"。它表示内存子系统的名称
const (
	memInfoSubsystem = "memory"
)

//定义了一个名为meminfoCollector的结构体类型，它包含一个logger字段，用于记录日志
type meminfoCollector struct {
	logger log.Logger
}

//定义一个名为init的函数，该函数在包初始化时自动执行
func init() {
	// 调用registerCollector函数，注册一个名为"meminfo"的收集器，使用defaultEnabled作为默认启用状态，
	// 并提供NewMeminfoCollector作为构造函数 
	registerCollector("meminfo", defaultEnabled, NewMeminfoCollector)
}

// NewMeminfoCollector returns a new Collector exposing memory stats.
// 定义了一个名为NewMeminfoCollector的函数，该函数接受一个logger作为参数，
// 返回一个Collector接口的实例和一个错误。它用于创建一个新的收集器实例 
func NewMeminfoCollector(logger log.Logger) (Collector, error) {
	return &meminfoCollector{logger}, nil
}

// Update calls (*meminfoCollector).getMemInfo to get the platform specific
// memory metrics.
// 定义了一个名为Update的方法，该方法接受一个类型为chan<- prometheus.Metric的通道ch，并返回一个error。它用于更新内存指标
func (c *meminfoCollector) Update(ch chan<- prometheus.Metric) error {
	var metricType prometheus.ValueType //定义变量metricType
	//调用getMemInfo方法，获取特定平台的内存指标信息，并将结果存储在memInfo变量中。如果有错误发生，将返回err
	memInfo, err := c.getMemInfo()
	if err != nil {
		return fmt.Errorf("couldn't get meminfo: %w", err)
	}
	level.Debug(c.logger).Log("msg", "Set node_mem", "memInfo", memInfo)
	for k, v := range memInfo { //遍历memInfo映射中的键值对。其中k表示字段名称，v表示对应的值
		//检查字段名称k是否以"_total"结尾，如果是，则将metricType设置为prometheus.CounterValue，表示计数器类型的指标；
		//否则，将metricType设置为prometheus.GaugeValue，表示仪表盘类型的指标
		if strings.HasSuffix(k, "_total") {
			metricType = prometheus.CounterValue
		} else {
			metricType = prometheus.GaugeValue
		}
		
		// 创建一个新的常量指标，并将其发送到通道ch中。
		// 指标的描述信息使用prometheus.BuildFQName函数构建，
                // 名称由namespace、memInfoSubsystem和字段名称k组成，用于标识该指标。
		// 指标的类型为metricType，值为v 
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, memInfoSubsystem, k),
				fmt.Sprintf("Memory information field %s.", k),
				nil, nil,
			),
			metricType, v,
		)
	}
	return nil //返回空值，表示没有发生错误
}
