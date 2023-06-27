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

package collector //定义了一个名为collector的包

// #include <mach/mach_host.h>
// #include <sys/sysctl.h>
// typedef struct xsw_usage xsw_usage_t;
import "C" //导入了C语言的代码

//导入Go语言包
import (
	"encoding/binary" //用于进行二进制数据的编解码
	"fmt" //提供格式化输入输出功能
	"unsafe" //包含了一些与Go语言的内存操作相关的不安全操作

	"golang.org/x/sys/unix" //提供了与Unix系统交互的功能
)

//定义了一个方法getMemInfo，属于meminfoCollector类型
func (c *meminfoCollector) getMemInfo() (map[string]float64, error) {
	host := C.mach_host_self()  //调用C函数mach_host_self()获取当前主机的句柄
	
	//定义一个变量infoCount，类型为mach_msg_type_number_t，并初始化为HOST_VM_INFO64_COUNT的值
	infoCount := C.mach_msg_type_number_t(C.HOST_VM_INFO64_COUNT)
	
	//定义一个变量vmstat，类型为vm_statistics64_data_t（定义不在给出的代码中），并初始化为空值
	vmstat := C.vm_statistics64_data_t{}

	//这部分代码用于获取总内存大小和交换区使用情况。
	//通过调用系统调用和使用Sysctl函数来获取主机的内存统计信息、总内存大小和交换区使用情况。
	//这些信息将在后续的代码中用于计算和构建内存信息的map。
	//如果获取这些信息的过程中发生错误，将返回相应的错误信息
	ret := C.host_statistics64( //调用C函数host_statistics64，获取主机的内存统计信息，并将结果存储在vmstat中
		C.host_t(host), //主机句柄，通过之前的C.mach_host_self()获取
		C.HOST_VM_INFO64, //表示要获取虚拟内存信息的类型
		C.host_info_t(unsafe.Pointer(&vmstat)), //传递了一个指向vmstat的指针，该指针通过unsafe.Pointer将Go语言的指针转换为C语言的指针类型
		&infoCount, //传递了一个指向infoCount的指针，用于指定接收虚拟内存信息的缓冲区的大小
	)
	//检查host_statistics64函数的返回值ret是否等于C.KERN_SUCCESS，如果不等于，则表示无法成功获取内存统计信息，将返回一个错误
	if ret != C.KERN_SUCCESS {
		return nil, fmt.Errorf("Couldn't get memory statistics, host_statistics returned %d", ret)
	}
	//使用unix.Sysctl函数获取系统的总内存大小，并将结果存储在totalb中
	totalb, err := unix.Sysctl("hw.memsize")
	if err != nil { //在Go语言中，nil表示一个空值或空指针
		return nil, err
	}

	//使用unix.SysctlRaw函数获取交换区使用情况的原始数据，并将结果存储在swapraw中
	swapraw, err := unix.SysctlRaw("vm.swapusage")
	if err != nil {
		return nil, err
	}
	//将swapraw的指针转换为C.xsw_usage_t类型的指针，以便访问交换区使用情况的结构体
	swap := (*C.xsw_usage_t)(unsafe.Pointer(&swapraw[0]))

	// Syscall removes terminating NUL which we need to cast to uint64
	//使用binary.LittleEndian.Uint64函数将totalb转换为无符号64位整数，并存储在total中。
	//这里在字符串totalb的末尾添加了一个空字符\x00，因为Sysctl函数会移除终止的空字符，而Uint64函数需要以NUL结尾的字节数组来解析为整数
	total := binary.LittleEndian.Uint64([]byte(totalb + "\x00"))

	var pageSize C.vm_size_t //定义一个变量pageSize，类型为vm_size_t
	C.host_page_size(C.host_t(host), &pageSize) //调用C函数host_page_size，获取主机的页面大小，并将结果存储在pageSize中

	ps := float64(pageSize) //将页面大小转换为浮点数类型，并存储在变量ps中
	//这段代码使用了C语言的系统调用和类型定义，与Go语言结合使用来获取主机的内存信息它通过调用C函数来获取主机的虚拟内存统计数据，
	//并使用Go语言的功能进行数据处理和转换。最后，将各个内存指标以字节数的形式构建成一个map，并作为函数的返回值
	return map[string]float64{ //构建一个map类型的内存信息，并作为函数的返回值。其中，键是内存指标的名称，值是对应的字节数
		"active_bytes":            ps * float64(vmstat.active_count),
		"compressed_bytes":        ps * float64(vmstat.compressor_page_count),
		"inactive_bytes":          ps * float64(vmstat.inactive_count),
		"wired_bytes":             ps * float64(vmstat.wire_count),
		"free_bytes":              ps * float64(vmstat.free_count),
		"swapped_in_bytes_total":  ps * float64(vmstat.pageins),
		"swapped_out_bytes_total": ps * float64(vmstat.pageouts),
		"internal_bytes":          ps * float64(vmstat.internal_page_count),
		"purgeable_bytes":         ps * float64(vmstat.purgeable_count),
		"total_bytes":             float64(total),
		"swap_used_bytes":         float64(swap.xsu_used),
		"swap_total_bytes":        float64(swap.xsu_total),
	}, nil
}
