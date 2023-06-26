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

//go:build !nomeminfo
// +build !nomeminfo

package collector //定义了一个名为collector的Go包

//导入了一些Go标准库，以及在其他文件中定义的一些内部包
import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

//定义了一个名为reParens的正则表达式变量，用于匹配括号中的内容
var (
	reParens = regexp.MustCompile(`\((.*)\)`)
)

//定义了getMemInfo方法，它接收无参数，并返回一个map[string]float64类型和一个error类型的值。
//该方法用于获取meminfo文件的内容，并将其解析为内存信息
func (c *meminfoCollector) getMemInfo() (map[string]float64, error) {
	//打开一个名为"meminfo"的文件，该文件位于Linux系统的/proc目录下，用于获取内存信息。如果有错误发生，将返回err
	file, err := os.Open(procFilePath("meminfo"))
	if err != nil {
		return nil, err
	}
	//在函数返回之前，延迟关闭file文件
	defer file.Close()

	return parseMemInfo(file) //调用parseMemInfo函数，将打开的文件file作为参数进行解析
}

//定义了parseMemInfo函数，它接收一个io.Reader类型的参数r，并返回一个map[string]float64类型的值和一个error类型的值。
//该函数用于解析从meminfo文件读取的内容，并将其转换为内存信息的键值对
func parseMemInfo(r io.Reader) (map[string]float64, error) {
	var (
		//定义了memInfo变量，类型为map[string]float64，用于存储解析后的内存信息。
		memInfo = map[string]float64{}
		//创建一个针对读取器r的bufio.Scanner扫描器
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() { //开始一个循环，循环遍历scanner扫描器读取的每一行
		line := scanner.Text() //获取当前行的文本内容，并将其存储在line变量中
		parts := strings.Fields(line) //使用strings.Fields函数将当前行拆分为多个字段，并将结果存储在parts字符串变量中
		// Workaround for empty lines occasionally occur in CentOS 6.2 kernel 3.10.90.
		if len(parts) == 0 { //检查字段的数量是否为0，如果是，则继续循环下一行。目的是处理空行
			continue
		}
		//将字段的第二个元素解析为float64类型的值，并将结果存储在fv变量中。如果有错误发生，将返回err
		fv, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value in meminfo: %w", err)
		}
		//将字段的第一个元素删除末尾的冒号，并将结果存储在key变量中
		key := parts[0][:len(parts[0])-1] // remove trailing : from key
		// Active(anon) -> Active_anon
		//使用正则表达式reParens替换key中的括号内容为_${1}。
		//这个替换的目的是将类似于"Active(anon)"的字段名称转换为"Active_anon"的形式
		key = reParens.ReplaceAllString(key, "_${1}")
		switch len(parts) {
		case 2: // no unit 如果字段数量为2，表示该行中的字段没有单位信息
		case 3: // has unit, we presume kB 如果字段数量为3，表示该行中的字段包含单位信息，这里假设单位为kB
			fv *= 1024 //将值fv乘以1024，将其转换为字节单位
			key = key + "_bytes" //将key末尾添加"_bytes"后缀，用于表示以字节为单位的指标
		default:
			return nil, fmt.Errorf("invalid line in meminfo: %s", line)
		}
		memInfo[key] = fv //将字段名称key作为键，对应的值fv作为值，存储到memInfo映射中
	}

	return memInfo, scanner.Err() //返回解析后的内存信息memInfo以及scanner扫描器的错误信息
}
