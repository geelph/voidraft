package version

// Version 版本注入 Ldflags
// 在项目最外层目录下的version.txt文件中修改，构建时会读取这个文件，并注入到Ldflags中
var Version = "0.0.0"
