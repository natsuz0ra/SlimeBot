!include "nsDialogs.nsh"

!macro customHeader
  !ifdef BUILD_UNINSTALLER
    ShowUninstDetails show
  !else
    ShowInstDetails show
  !endif
!macroend

!ifndef BUILD_UNINSTALLER
Var desktopShortcutChoice
Var desktopShortcutCheckbox

!macro customInit
  ; Silent installs and upgrades retain electron-builder's existing shortcut behavior.
  StrCpy $desktopShortcutChoice 1
!macroend

!macro customPageAfterChangeDir
  Page custom desktopShortcutPageCreate desktopShortcutPageLeave
  !define MUI_PAGE_CUSTOMFUNCTION_SHOW installFilesPageShow
!macroend

Function desktopShortcutPageCreate
  ${if} ${isUpdated}
    Abort
  ${endIf}

  nsDialogs::Create 1018
  Pop $0
  ${if} $0 == error
    Abort
  ${endIf}

  !insertmacro MUI_HEADER_TEXT "安装选项 / Install options" "选择需要的快捷方式 / Choose a shortcut"
  ${NSD_CreateLabel} 0 0 100% 24u "开始安装前，可选择是否在桌面创建 SlimeBot 快捷方式。$\r$\nChoose whether to create a SlimeBot desktop shortcut."
  Pop $0
  ${NSD_CreateCheckbox} 0 34u 100% 18u "创建桌面快捷方式 / Create desktop shortcut"
  Pop $desktopShortcutCheckbox
  ${if} $desktopShortcutChoice == 1
    ${NSD_Check} $desktopShortcutCheckbox
  ${endIf}
  nsDialogs::Show
FunctionEnd

Function desktopShortcutPageLeave
  ${NSD_GetState} $desktopShortcutCheckbox $desktopShortcutChoice
FunctionEnd

Function installFilesPageShow
  SetDetailsPrint both
  DetailPrint "正在复制 SlimeBot 程序文件 / Copying SlimeBot application files"
FunctionEnd

!macro customInstall
  SetDetailsPrint both
  DetailPrint "程序文件已复制到 $INSTDIR / Application files copied"
  ${if} $desktopShortcutChoice == 0
    DetailPrint "未选择桌面快捷方式，正在移除 / Removing unselected desktop shortcut"
    WinShell::UninstShortcut "$newDesktopLink"
    Delete "$newDesktopLink"
    ${if} $oldDesktopLink != $newDesktopLink
      WinShell::UninstShortcut "$oldDesktopLink"
      Delete "$oldDesktopLink"
    ${endIf}
  ${else}
    DetailPrint "桌面快捷方式已处理 / Desktop shortcut ready"
  ${endIf}
  DetailPrint "安装完成 / Installation complete"
!macroend

!endif

!macro customUnInstall
  SetDetailsPrint both
  DetailPrint "正在卸载 SlimeBot / Uninstalling SlimeBot"
  DetailPrint "正在移除 $INSTDIR 中的程序文件 / Removing application files"
!macroend
