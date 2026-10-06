package browser

import "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/contract"

// secureContractFilePath 校验请求里不可信的文件名，返回保证落在 baseDir 内的绝对路径。
//
// 这些名字直接来自请求 JSON（SourceFile.FileName、MateDataFile.FileName、
// HardhatBuildInfoFile.FileName），历史上被原样拼成 "fileDir + name"，允许用 ".."
// 逃出目录并在主机上写任意文件（/api 组无鉴权，匿名可打）。
//
// 校验逻辑集中在 contract.SecureJoin（同族漏洞——CompileWithMetaData 里以用户提交的
// metadata.Sources 键作落盘路径——也走同一个函数），这里只做转发，避免两处实现漂移。
// 语义：**保留合法的相对子目录**（contracts/Foo.sol 仍写到 <baseDir>/contracts/Foo.sol），
// 只拒绝越界形态；任一违规返回错误（fail-closed）。
func secureContractFilePath(baseDir, fileName string) (string, error) {
	return contract.SecureJoin(baseDir, fileName)
}
