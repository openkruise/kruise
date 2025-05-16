# **为 OpenKruise 设计模糊测试并与 OSS-Fuzz 集成**

[English](./README.md) | 简体中文

## **1. 背景与目标**

### **1.1 模糊测试简介**

模糊测试（Fuzz Testing）是一种自动化测试方法，通过向程序输入大量随机或变异的数据，检测边界条件错误、内存泄漏、逻辑漏洞等问题。其核心价值在于：

- **自动发现潜在漏洞**（如缓冲区溢出、空指针）
- **验证输入处理逻辑的健壮性**
- **持续保障代码质量**（集成到 CI/CD 流程）

### **1.2 OpenKruise 与 OSS-Fuzz**

- **[OSS-Fuzz](https://github.com/google/oss-fuzz)** 是 Google 开源的持续模糊测试平台，支持自动构建、运行模糊测试并报告漏洞。
- **目标**：为 OpenKruise 的关键功能设计模糊测试用例，并集成到 OSS-Fuzz 平台，实现持续安全防护。


## **2. 模糊测试设计**

### **2.1 选择测试目标**

优先选择以下类型函数作为模糊测试目标：

- **输入解析函数**
  - 处理外部输入（如用户输入、文件、网络数据）。
  - 涉及复杂格式解析（如 JSON/YAML/XML、二进制协议、自定义格式）。
  - 容易因格式错误导致崩溃或内存问题。

### **2.2 原生 Go 1.18 模糊测试**

Go 1.18 原生支持模糊测试，适合简单输入场景。

#### **示例代码**

```go
package kruise_test

import (
  "testing"
)

// 目标函数：解析 YAML 字符串
func ParseYAML(input string) (interface{}, error) {
  // 实际实现
}

// 模糊测试函数
func FuzzParseYAML(f *testing.F) {
  // f.Add()
  f.Fuzz(func(t *testing.T, data []byte) {
    _, err := ParseYAML(string(data))
    if err != nil {
      t.Logf("Error parsing YAML: %v", err)
    }
  })
}
```

#### **关键点**

- `f.Add` 用来添加种子输入。
- `f.Fuzz` 自动生成随机输入（基于种子）并调用目标函数。
- 通过 `t.Logf` 记录异常情况（如解析失败）。
- 通过 `t.Errorf` 可以中断执行并记录异常情况。
- 同时也可以忽略错误以检查目标函数是否会崩溃。


### **2.3 第三方库增强模糊测试**

对于复杂结构体的模糊测试，推荐使用 [`go-fuzz-headers`](https://github.com/AdaLogics/go-fuzz-headers)。

#### **示例代码**

```go
package kruise_test

import (
  "testing"
  "github.com/AdaLogics/go-fuzz-headers/fuzz"
)

// 目标函数：处理 Pod 生命周期事件
func HandlePodEvent(event *corev1.Pod) error {
  // 实际实现
}

// 模糊测试函数
func FuzzHandlePodEvent(f *testing.F) {
  f.Fuzz(func(t *testing.T, data []byte) {
    cf := fuzz.NewConsumer(data)
    pod := corev1.Pod{}
    if err := cf.GenerateStruct(&pod); err != nil {
      return
    }
    _ = HandlePodEvent(&pod)
  })
}
```

#### **优势**

- 通过 `cf.GenerateStruct `函数可以直接模糊整个结构体 `corev1.Pod` 的字段。
- 同时也支持嵌套字段的深度模糊测试。


## **3. 与 OSS-Fuzz 集成**

### **3.1 目录结构规范**

在 OpenKruise 项目中，编写好的模糊测试需添加到以下脚本与 [OSS-Fuzz](https://github.com/google/oss-fuzz) 集成：

```
openkruise/test/fuzz/oss_fuzz_build.sh 
```

### **3.2 编写编译脚本**

修改 `oss_fuzz_build.sh`，添加模糊测试目标：

```
compile_native_go_fuzzer <fuzz_path> <fuzz_func> <fuzz_target>
```


## **4. 本地验证与调试**

### **4.1 基于 OSS-Fuzz 本地运行模糊测试**

1. 下载代码：

```
$ git clone https://github.com/google/oss-fuzz.git
$ cd oss-fuzz
```

2. 构建镜像：

```
$ python infra/helper.py build_image openkruise
```

3. 编译：

```
$ python infra/helper.py build_fuzzers openkruise
```

4. 针对特定模糊测试目标执行测试：

```
$ python infra/helper.py run_fuzzer openkruise <fuzz_target>
```

### **4.2 运行待合并的模糊测试**

通过 fork [OSS-Fuzz](https://github.com/google/oss-fuzz) 仓库，修改 `oss-fuzz/projects/openkruise/Dockerfile` 中的仓库路径，用来测试还未合并到 `openkruise/master` 的模糊测试，例如：

```
RUN git clone --branch <your_fork_openkruise_branch> --depth 1 <your_fork_openkruise_repo>
```

### **4.3 复现 Bug**

若发现崩溃， [OSS-Fuzz](https://github.com/google/oss-fuzz) 会在 `/build/out/openkruise/` 下生成  `crash-` 开头的文件。修复后可通过以下命令验证：

```
$ python infra/helper.py reproduce openkruise <fuzz_target> <testcase_path>
```

## **5. 常见问题**

### **Q1: 如何收到 OSS-Fuzz 提供的测试报告？**

- 在 `oss-fuzz/projects/openkruise/project.yaml` 的 `auto_ccs` 列表中添加邮箱。
- 添加完成以后可以提交 Pull Request 待 [OSS-Fuzz](https://github.com/google/oss-fuzz) 社区合并即可。

### **Q2: 如何提升模糊测试覆盖率？**

- 为了提升模糊测试的覆盖率，可以手动构建针对特定逻辑或者语义正确的测试用例。


## **6. 参考资料**

- [OSS-Fuzz 新项目指南](https://google.github.io/oss-fuzz/getting-started/new-project-guide/go-lang/)
- [Tutorial: Getting started with fuzzing](https://go.dev/doc/tutorial/fuzz)
- [go-fuzz-headers GitHub 仓库](https://github.com/AdaLogics/go-fuzz-headers)