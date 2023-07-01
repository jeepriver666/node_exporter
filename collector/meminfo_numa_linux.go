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

//go:build !nomeminfo_numa
// +build !nomeminfo_numa

package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

//定义了一个常量 memInfoNumaSubsystem，它的值是字符串 "memory_numa"。
//这个常量表示内存统计信息的子系统名称
const (
	memInfoNumaSubsystem = "memory_numa"
)

//定义了一个变量 meminfoNodeRE，它是一个正则表达式对象。
//这个正则表达式用于匹配节点路径中的节点号。
//它会匹配类似于 "devices/system/node/node1" 的路径，
//并提取出节点号（在这个例子中是数字 1）
var meminfoNodeRE = regexp.MustCompile(`.*devices/system/node/node([0-9]*)`)

//定义了一个名为 meminfoMetric 的结构体类型。
//这个结构体用于存储内存指标的相关信息，包括指标名称、指标类型、节点号和数值
type meminfoMetric struct {
	metricName string
	metricType prometheus.ValueType
	numaNode   string
	value      float64
}

//定义了一个名为 meminfoNumaCollector 的结构体类型。
//这个结构体表示一个内存统计收集器，包含了存储指标描述符的映射和一个日志记录器
type meminfoNumaCollector struct {
	metricDescs map[string]*prometheus.Desc
	logger      log.Logger
}

//这是一个初始化函数 init，它在包被导入时自动执行。
//它调用了一个名为 registerCollector 的函数，
//将收集器的名称、默认禁用状态和 NewMeminfoNumaCollector 函数作为参数传递给它。
//这个函数的作用是注册内存统计收集器
func init() {
	registerCollector("meminfo_numa", defaultDisabled, NewMeminfoNumaCollector)
}

//这是一个构造函数 NewMeminfoNumaCollector，它返回一个新的内存统计收集器。
//它接收一个日志记录器作为参数，并返回一个实现了 Collector 接口的对象。
//在这个函数中，创建了一个新的 meminfoNumaCollector 对象，
//其中的 metricDescs 字段被初始化为空的映射，而 logger 字段则被设置为传入的日志记录器
// NewMeminfoNumaCollector returns a new Collector exposing memory stats.
func NewMeminfoNumaCollector(logger log.Logger) (Collector, error) {
	return &meminfoNumaCollector{
		metricDescs: map[string]*prometheus.Desc{},
		logger:      logger,
	}, nil
}

//这是 meminfoNumaCollector 结构体的一个方法 Update。
//它实现了 Collector 接口中的 Update 方法。
//这个方法用于更新收集器中的指标，并将其发送到传入的通道 ch 中。
func (c *meminfoNumaCollector) Update(ch chan<- prometheus.Metric) error {
	metrics, err := getMemInfoNuma() //调用 getMemInfoNuma 函数获取内存统计信息，并将结果保存在 metrics 变量中。
	if err != nil {
		return fmt.Errorf("couldn't get NUMA meminfo: %w", err)
	}
	for _, v := range metrics { //遍历指标
		////根据指标名称从 metricDescs 字段中获取相应的指标描述符 desc
		desc, ok := c.metricDescs[v.metricName]
		if !ok {
			//如果 desc 不存在，则创建一个新的指标描述符，并将其存储在 metricDescs 中
			desc = prometheus.NewDesc(
				prometheus.BuildFQName(namespace, memInfoNumaSubsystem, v.metricName),
				fmt.Sprintf("Memory information field %s.", v.metricName),
				[]string{"node"}, nil)
			c.metricDescs[v.metricName] = desc
		}
		//使用 desc 和指标的类型、数值和节点号创建一个常量指标，并将其发送到通道 ch 中
		ch <- prometheus.MustNewConstMetric(desc, v.metricType, v.value, v.numaNode)
	}
	return nil
}

//这是 getMemInfoNuma 函数，用于获取内存统计信息。
//它首先使用 filepath.Glob 函数查找匹配模式 "devices/system/node/node[0-9]*" 的节点路径。
//然后，它打开每个节点的 meminfo 文件和 numastat 文件，
//并分别调用 parseMemInfoNuma 和 parseMemInfoNumaStat 函数解析这些文件的内容。
//最后，它将解析得到的指标信息添加到 metrics 切片中，并返回该切片
func getMemInfoNuma() ([]meminfoMetric, error) {
	var (
		metrics []meminfoMetric
	)

	nodes, err := filepath.Glob(sysFilePath("devices/system/node/node[0-9]*"))
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		meminfoFile, err := os.Open(filepath.Join(node, "meminfo"))
		if err != nil {
			return nil, err
		}
		defer meminfoFile.Close()

		numaInfo, err := parseMemInfoNuma(meminfoFile)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, numaInfo...)

		numastatFile, err := os.Open(filepath.Join(node, "numastat"))
		if err != nil {
			return nil, err
		}
		defer numastatFile.Close()

		nodeNumber := meminfoNodeRE.FindStringSubmatch(node)
		if nodeNumber == nil {
			return nil, fmt.Errorf("device node string didn't match regexp: %s", node)
		}

		numaStat, err := parseMemInfoNumaStat(numastatFile, nodeNumber[1])
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, numaStat...)
	}

	return metrics, nil
}

//这是 parseMemInfoNuma 函数，用于解析 meminfo 文件的内容。
//它接收一个实现了 io.Reader 接口的参数 r，并返回解析得到的指标信息。
//函数中首先创建一个空的指标切片 memInfo，然后使用 bufio.NewScanner 函数创建一个扫描器。
//接下来，它定义了一个正则表达式对象 re，用于匹配括号中的内容。
//然后，它开始逐行扫描输入。对于每一行，它首先去除首尾的空白字符，并判断是否为空行，
//如果是则继续下一行的扫描。然后，它将行按空白字符分割成多个部分，并将第四个部分解析为浮点数 fv。
//根据部分的数量，它判断是否有单位（如果有单位，则将数值乘以 1024）。
//然后，它对指标名称进行处理，将括号中的内容替换为下划线。
//最后，它将解析得到的指标信息添加到 memInfo 切片中，并继续下一行的扫描。
//最后，它返回 memInfo 切片和扫描器的错误
func parseMemInfoNuma(r io.Reader) ([]meminfoMetric, error) {
	var (
		memInfo []meminfoMetric
		scanner = bufio.NewScanner(r)
		re      = regexp.MustCompile(`\((.*)\)`)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)

		fv, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value in meminfo: %w", err)
		}
		switch l := len(parts); {
		case l == 4: // no unit
		case l == 5 && parts[4] == "kB": // has unit
			fv *= 1024
		default:
			return nil, fmt.Errorf("invalid line in meminfo: %s", line)
		}
		metric := strings.TrimRight(parts[2], ":")

		// Active(anon) -> Active_anon
		metric = re.ReplaceAllString(metric, "_${1}")
		memInfo = append(memInfo, meminfoMetric{metric, prometheus.GaugeValue, parts[1], fv})
	}

	return memInfo, scanner.Err()
}

//这是 parseMemInfoNumaStat 函数，用于解析 numastat 文件的内容。
//它与 parseMemInfoNuma 函数类似，接收一个实现了 io.Reader 接口的参数 r 和一个节点号 nodeNumber，
//并返回解析得到的指标信息。函数中首先创建一个空的指标切片 numaStat，
//然后使用 bufio.NewScanner 函数创建一个扫描器。接下来，它开始逐行扫描输入。
//对于每一行，它首先去除首尾的空白字符，并判断是否为空行，如果是则继续下一行的扫描。
//然后，它将行按空白字符分割成多个部分，并将第二个部分解析为浮点数 fv。
//然后，它将指标名称和节点号添加到 numaStat 切片中，并继续下一行的扫描。
//最后，它返回 numaStat 切片和扫描器的错误
func parseMemInfoNumaStat(r io.Reader, nodeNumber string) ([]meminfoMetric, error) {
	var (
		numaStat []meminfoMetric
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line scan did not return 2 fields: %s", line)
		}

		fv, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value in numastat: %w", err)
		}

		numaStat = append(numaStat, meminfoMetric{parts[0] + "_total", prometheus.CounterValue, nodeNumber, fv})
	}
	return numaStat, scanner.Err()
}
