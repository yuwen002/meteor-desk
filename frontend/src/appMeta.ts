import packageJson from '../package.json'

// 版本号只在 frontend/package.json 维护，界面统一从这里读取。
export const appVersion = packageJson.version
export const repositoryURL = 'https://github.com/yuwen002/meteor-desk'
