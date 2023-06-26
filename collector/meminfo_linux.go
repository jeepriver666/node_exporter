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

var (
	reParens = regexp.MustCompile(`\((.*)\)`)
)

func (c *meminfoCollector) getMemInfo() (map[string]float64, error) {
	//打开一个名为"meminfo"的文件，该文件位于Linux系统的/proc目录下，用于获取内存信息。如果有错误发生，将返回err
	file, err := os.Open(procFilePath("meminfo"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseMemInfo(file)
}

func parseMemInfo(r io.Reader) (map[string]float64, error) {
	var (
		memInfo = map[string]float64{}
		//创建一个针对打开的文件的扫描器
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() { //开始一个循环，扫描器每次读取文件的一行
		line := scanner.Text()
		parts := strings.Fields(line) //使用strings.Fields函数将当前行分割成多个字段，存储在名为parts的字符串切片中
		// Workaround for empty lines occasionally occur in CentOS 6.2 kernel 3.10.90.
		if len(parts) == 0 { //检查字段的数量是否为0，如果是，则继续循环下一行
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
		key = reParens.ReplaceAllString(key, "_${1}")
		switch len(parts) {
		case 2: // no unit
		case 3: // has unit, we presume kB
			fv *= 1024
			key = key + "_bytes"
		default:
			return nil, fmt.Errorf("invalid line in meminfo: %s", line)
		}
		memInfo[key] = fv
	}

	return memInfo, scanner.Err()
}
