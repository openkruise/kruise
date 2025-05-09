# **Design Fuzz Testing for OpenKruise and Integrate with OSS-Fuzz**

English | [简体中文](./README-zh_CN.md)

## **1. Background and Objectives**

### **1.1 Fuzz Testing Overview**

Fuzz testing is an automated testing method that injects random or mutated data into a program to detect boundary condition errors, memory leaks, and logical vulnerabilities. Its core value includes:

- **Automatically discovering potential vulnerabilities** (e.g., buffer overflows, null pointer dereferences)
- **Validating the robustness of input handling logic**
- **Continuously ensuring code quality** (integrated into CI/CD pipelines)

### **1.2 OpenKruise and OSS-Fuzz**

- **[OSS-Fuzz](https://github.com/google/oss-fuzz)** is Google's open-source continuous fuzz testing platform, supporting automated build, execution of fuzz tests, and vulnerability reporting.
- **Objective**: Design fuzz test cases for key OpenKruise functionalities and integrate them into the OSS-Fuzz platform to achieve continuous security protection.


## **2. Fuzz Test Design**

### **2.1 Selecting Test Targets**

Prioritize the following types of functions as fuzz testing targets:

- Input parsing functions
  - Process external inputs (e.g., user input, files, network data).
  - Involve complex format parsing (e.g., JSON/YAML/XML, binary protocols, custom formats).
  - Likely to crash or cause memory issues due to format errors.

### **2.2 Native Go 1.18 Fuzz Testing**

Go 1.18 natively supports fuzz testing, suitable for simple input scenarios.

#### **Example Code**

```go
package kruise_test

import (
  "testing"
)

// Target function: Parse YAML string
func ParseYAML(input string) (interface{}, error) {
  // Actual implementation
}

// Fuzz test function
func FuzzParseYAML(f *testing.F) {
  f.Fuzz(func(t *testing.T, data []byte) {
    _, err := ParseYAML(string(data))
    if err != nil {
      t.Logf("Error parsing YAML: %v", err)
    }
  })
}
```

- Key Points:
  - `f.Add` adds seed inputs.
  - `f.Fuzz` generates random inputs (based on seeds) and invokes the target function.
  - Use `t.Logf` to log anomalies (e.g., parsing failures).
  - Use `t.Errorf` to interrupt execution and log errors.
  - Ignore errors to check if the target function crashes.

### **2.3 Enhancing Fuzz Testing with Third-Party Libraries**

For complex structure fuzz testing, recommend using [`go-fuzz-headers`](https://github.com/AdaLogics/go-fuzz-headers).

#### **Example Code**

```go
package kruise_test

import (
  "testing"
  "github.com/AdaLogics/go-fuzz-headers/fuzz"
)

// Target function: Handle Pod lifecycle events
func HandlePodEvent(event *corev1.Pod) error {
  // Actual implementation
}

// Fuzz test function
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

- Advantages:
  - Directly fuzz fields of complex structures (e.g., `corev1.Pod`) using `cf.GenerateStruct`.
  - Supports deep fuzz testing of nested fields.


## **3. Integration with OSS-Fuzz**

### **3.1 Directory Structure Specifications**

In the OpenKruise project, completed fuzz tests must be added to the following script for integration with OSS-Fuzz:

```
openkruise/test/fuzz/oss_fuzz_build.sh 
```

### **3.2 Writing the Compilation Script**

Modify `oss_fuzz_build.sh` to add fuzz test targets:

```
compile_native_go_fuzzer <fuzz_path> <fuzz_func> <fuzz_target>
```


## **4. Local Validation and Debugging**

### **4.1 Running Fuzz Tests Locally with OSS-Fuzz**

1. Git clone:

 ```
 $ git clone https://github.com/google/oss-fuzz.git
 $ cd oss-fuzz
 ```

2. Build the image:

```
$ python infra/helper.py build_image openkruise
```

3. Compile fuzz tests:

 ```
 $ python infra/helper.py build_fuzzers openkruise
 ```

4. Run tests for specific targets:

 ```
 $ python infra/helper.py run_fuzzer openkruise <fuzz_target>
 ```

### **4.2 Testing Unmerged Fuzz Tests**

Fork the [OSS-Fuzz](https://github.com/google/oss-fuzz) repository and modify `oss-fuzz/projects/openkruise/Dockerfile` to test fuzz tests not yet merged into `openkruise/master`

```
RUN git clone --branch <your_fork_openkruise_branch> --depth 1 <your_fork_openkruise_repo>
```

### **4.3 Reproducing Bugs**

If crashes are detected, OSS-Fuzz generates files starting with `crash-` in `/build/out/openkruise/`. After fixing bugs, verify with:

```
$ python infra/helper.py reproduce openkruise <fuzz_target> <testcase_path>
```


## **5. Common Issues**

### **Q1: How to receive OSS-Fuzz test reports?**

- Add email addresses to the `auto_ccs` list in `oss-fuzz/projects/openkruise/project.yaml`.
- Submit a Pull Request for the  [OSS-Fuzz](https://github.com/google/oss-fuzz)  community to merge.

### **Q2: How to improve fuzz test coverage?**

- To enhance the coverage of fuzz testing, one can manually construct test cases that target specific logic or are semantically correct.


## **6. References**

- [OSS-Fuzz New Project Guide](https://google.github.io/oss-fuzz/getting-started/new-project-guide/go-lang/)
- [Tutorial: Getting Started with Fuzzing](https://go.dev/doc/tutorial/fuzz)
- [go-fuzz-headers GitHub Repository](https://github.com/AdaLogics/go-fuzz-headers)