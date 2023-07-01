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
//该函数返回一个[]meminfoMetric类型的切片和一个error类型的错误对象
func getMemInfoNuma() ([]meminfoMetric, error) {
	//声明一个名为metrics的空切片，用于存储内存信息
	var (
		metrics []meminfoMetric
	)

        //使用 filepath.Glob 函数查找匹配模式 "devices/system/node/node[0-9]*" 的文件路径
	//该模式用于找到NUMA节点。如果发生错误，则返回nil和错误对象
	nodes, err := filepath.Glob(sysFilePath("devices/system/node/node[0-9]*"))
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		//打开每个节点的 meminfo 文件，将其赋值给meminfoFile变量
		//如果发生错误，则返回nil和错误对象
		meminfoFile, err := os.Open(filepath.Join(node, "meminfo"))
		if err != nil {
			return nil, err
		}
		//使用defer关键字延迟关闭meminfoFile，确保它在后续代码执行完毕后被关闭
		defer meminfoFile.Close()

		//调用parseMemInfoNuma()函数，传递meminfoFile作为参数，以解析文件中的内存信息。
		//将返回值赋给numaInfo变量。如果发生错误，则返回nil和错误对象
		numaInfo, err := parseMemInfoNuma(meminfoFile)
		if err != nil {
			return nil, err
		}
		
		//使用append()函数将numaInfo中的所有元素追加到metrics切片中
		metrics = append(metrics, numaInfo...)

		//打开每个节点的 numastat 文件，并将其赋值给numastatFile变量。
		//如果发生错误，则返回nil和错误对象
		numastatFile, err := os.Open(filepath.Join(node, "numastat"))
		if err != nil {
			return nil, err
		}
		//使用defer关键字延迟关闭numastatFile
		defer numastatFile.Close()

		//使用正则表达式（meminfoNodeRE.FindStringSubmatch(node)）从当前节点路径中提取节点编号。
		//将提取的节点编号存储在nodeNumber变量中。
		//如果提取失败（nodeNumber == nil），则返回错误对象，指示设备节点字符串与正则表达式不匹配
		nodeNumber := meminfoNodeRE.FindStringSubmatch(node)
		if nodeNumber == nil {
			return nil, fmt.Errorf("device node string didn't match regexp: %s", node)
		}

		//调用parseMemInfoNumaStat()函数，传递numastatFile和提取的节点编号nodeNumber[1]作为参数，
		//以解析文件中的NUMA统计信息。将返回值赋给numaStat变量。
		//如果发生错误，则返回nil和错误对象
		numaStat, err := parseMemInfoNumaStat(numastatFile, nodeNumber[1])
		if err != nil {
			return nil, err
		}
		//使用append()函数将numaStat中的所有元素追加到metrics切片中
		metrics = append(metrics, numaStat...)
	}

	//在遍历完所有NUMA节点后，返回包含所有内存信息的metrics切片，并返回nil错误，表示成功
	return metrics, nil
}

//这是 parseMemInfoNuma 函数，用于解析 meminfo 文件的内容。
//它接收一个实现了 io.Reader 接口的参数 r，并返回解析得到的指标信息。
func parseMemInfoNuma(r io.Reader) ([]meminfoMetric, error) {
	var (
		memInfo []meminfoMetric //创建一个空的指标切片 memInfo
		//使用 bufio.NewScanner 函数创建一个bufio.Scanner对象，用于逐行读取输入的内容
		scanner = bufio.NewScanner(r)
		re      = regexp.MustCompile(`\((.*)\)`) //定义了一个正则表达式对象re，用于匹配括号中的内容
	)

	for scanner.Scan() { //通过scanner.Scan()循环读取每一行的内容,逐行扫描输入
		//使用strings.TrimSpace()函数去除行首和行尾的空白字符，并将结果赋给line变量
		line := strings.TrimSpace(scanner.Text())
		if line == "" { //如果line为空字符串，则跳过当前循环
			continue
		}
		//使用strings.Fields()函数将line按空白字符分割为多个部分，并将结果赋给parts变量
		parts := strings.Fields(line)

		//将parts[3]转换为float64类型的数值，并将结果赋给fv变量
		fv, err := strconv.ParseFloat(parts[3], 64)
		if err != nil { //如果转换过程中发生错误，则返回错误对象，指示在meminfo中存在无效的数值
			return nil, fmt.Errorf("invalid value in meminfo: %w", err)
		}
		switch l := len(parts); { //根据parts的长度进行不同的处理
		case l == 4: // no unit 如果parts长度为4，则表示没有单位，不进行任何处理
		case l == 5 && parts[4] == "kB": // has unit 如果parts长度为5且parts[4]等于"kB"，则表示有单位
			fv *= 1024 //将fv乘以1024转换为字节
		default: //否则，返回错误对象，指示在meminfo中存在无效的行
			return nil, fmt.Errorf("invalid line in meminfo: %s", line)
		}
		//去除parts[2]末尾的冒号，并将结果赋给metric变量
		metric := strings.TrimRight(parts[2], ":")

		// Active(anon) -> Active_anon
		//使用正则表达式re替换metric中的括号内容，将括号内的内容替换为_${1}。
		metric = re.ReplaceAllString(metric, "_${1}")
		//将metric、prometheus.GaugeValue、parts[1]和fv作为字段值，创建一个meminfoMetric结构体，
		//并将其追加到memInfo切片中
		memInfo = append(memInfo, meminfoMetric{metric, prometheus.GaugeValue, parts[1], fv})
	}

	//返回存储解析后的内存信息的memInfo切片，并返回scanner.Err()，表示解析过程中的错误（如果有）
	return memInfo, scanner.Err()
}

//这是 parseMemInfoNumaStat 函数，用于解析 numastat 文件的内容。
//它与 parseMemInfoNuma 函数类似，接收一个实现了 io.Reader 接口的参数 r 和一个节点号 nodeNumber，
//并返回解析得到的指标信息，返回一个[]meminfoMetric类型的切片和一个error类型的错误对象。
func parseMemInfoNumaStat(r io.Reader, nodeNumber string) ([]meminfoMetric, error) {
	var (
		numaStat []meminfoMetric //创建一个空的指标切片 numaStat
		//使用 bufio.NewScanner 函数创建一个bufio.Scanner对象，用于逐行读取输入的内容
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() { //通过scanner.Scan()循环读取每一行的内容
		//使用strings.TrimSpace()函数去除行首和行尾的空白字符，并将结果赋给line变量
		line := strings.TrimSpace(scanner.Text())
		if line == "" { //如果line为空字符串，则跳过当前循环
			continue
		}
		//使用strings.Fields()函数将line按空白字符分割为多个部分，并将结果赋给parts变量
		parts := strings.Fields(line)
		if len(parts) != 2 { //如果parts的长度不等于2，则返回错误对象，指示行扫描没有返回2个字段
			return nil, fmt.Errorf("line scan did not return 2 fields: %s", line)
		}

		//将parts[1]转换为float64类型的数值，并将结果赋给fv变量
		fv, err := strconv.ParseFloat(parts[1], 64)
		if err != nil { //如果转换过程中发生错误，则返回错误对象，指示在numastat中存在无效的数值
			return nil, fmt.Errorf("invalid value in numastat: %w", err)
		}

		//使用append()函数将包含节点编号的统计信息添加到numaStat切片中，
		//其中字段名称为parts[0] + "_total"，指标类型为prometheus.CounterValue，节点编号为nodeNumber，值为fv
		numaStat = append(numaStat, meminfoMetric{parts[0] + "_total", prometheus.CounterValue, nodeNumber, fv})
	}
	
	//返回存储解析后的NUMA统计信息的numaStat切片，并返回scanner.Err()，表示解析过程中的错误（如果有）
	return numaStat, scanner.Err()
}
