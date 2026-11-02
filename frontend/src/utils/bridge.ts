// 桌面桥接的唯一读取点：window.go.main.App 只在 Wails 运行时里存在，
// 收敛成单个函数后，App.vue 与各 composable 不必重复写可选链，
// 单元测试也只需要给 window.go 注入桩对象。
export function getBridge() {
  return window.go?.main?.App
}
