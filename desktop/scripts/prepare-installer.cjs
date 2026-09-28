const fs = require('node:fs')
const path = require('node:path')

const templates = path.join(path.dirname(require.resolve('app-builder-lib/package.json')), 'templates', 'nsis')

function replaceTemplate(relativePath, before, after) {
  const file = path.join(templates, relativePath)
  const source = fs.readFileSync(file, 'utf8')
  if (!source.includes(before)) {
    if (source.includes(after)) return
    throw new Error(`Unsupported electron-builder NSIS template: ${relativePath}`)
  }
  fs.writeFileSync(file, source.replaceAll(before, after))
}

replaceTemplate('installSection.nsh', '  SetDetailsPrint none', '  SetDetailsPrint both')
replaceTemplate('include/extractAppPackage.nsh',
  'Nsis7z::Extract "${FILE}"',
  'Nsis7z::ExtractWithDetails "${FILE}" "正在解压 / Extracting %s"')
replaceTemplate('include/extractAppPackage.nsh',
  '    CopyFiles /SILENT "$PLUGINSDIR\\7z-out\\*" $OUTDIR',
  '    DetailPrint "正在复制文件到 $OUTDIR / Copying files to $OUTDIR"\n    CopyFiles /SILENT "$PLUGINSDIR\\7z-out\\*" $OUTDIR')
