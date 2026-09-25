$ErrorActionPreference = 'Stop'

try {
    $req = $env:SLIMEBOT_COMPUTER_REQUEST | ConvertFrom-Json
    Add-Type -AssemblyName UIAutomationClient
    Add-Type -AssemblyName UIAutomationTypes
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
public static class SlimeBotDesktopInput {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll", SetLastError = true)] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extraInfo);

    [StructLayout(LayoutKind.Sequential)]
    public struct Input { public uint type; public InputUnion data; }
    [StructLayout(LayoutKind.Explicit)]
    public struct InputUnion {
        [FieldOffset(0)] public MouseInput mouse;
        [FieldOffset(0)] public KeyboardInput keyboard;
    }
    [StructLayout(LayoutKind.Sequential)]
    public struct MouseInput {
        public int dx, dy;
        public uint mouseData, flags, time;
        public UIntPtr extraInfo;
    }
    [StructLayout(LayoutKind.Sequential)]
    public struct KeyboardInput {
        public ushort virtualKey, scanCode;
        public uint flags, time;
        public UIntPtr extraInfo;
    }
    [DllImport("user32.dll", SetLastError = true)]
    private static extern uint SendInput(uint count, Input[] inputs, int size);

    private static void Dispatch(Input[] inputs) {
        uint sent = SendInput((uint)inputs.Length, inputs, Marshal.SizeOf(typeof(Input)));
        if (sent != inputs.Length) throw new Win32Exception(Marshal.GetLastWin32Error(), "SendInput failed");
    }

    public static void TypeText(string text) {
        for (int start = 0; start < text.Length; start += 128) {
            int count = Math.Min(128, text.Length - start);
            var inputs = new Input[count * 2];
            for (int i = 0; i < count; i++) {
                ushort code = text[start + i];
                inputs[i * 2].type = 1;
                inputs[i * 2].data.keyboard.scanCode = code;
                inputs[i * 2].data.keyboard.flags = 0x0004;
                inputs[i * 2 + 1].type = 1;
                inputs[i * 2 + 1].data.keyboard.scanCode = code;
                inputs[i * 2 + 1].data.keyboard.flags = 0x0006;
            }
            Dispatch(inputs);
        }
    }

    public static void PressKey(ushort virtualKey) {
        var inputs = new Input[2];
        inputs[0].type = 1;
        inputs[0].data.keyboard.virtualKey = virtualKey;
        inputs[1].type = 1;
        inputs[1].data.keyboard.virtualKey = virtualKey;
        inputs[1].data.keyboard.flags = 0x0002;
        Dispatch(inputs);
    }
}
'@
    [void][SlimeBotDesktopInput]::SetProcessDPIAware()

    function Get-FocusedWindow {
        $focused = [System.Windows.Automation.AutomationElement]::FocusedElement
        if ($null -eq $focused) { throw 'No focused Windows UI element is available.' }
        $processId = $focused.Current.ProcessId
        $root = [System.Windows.Automation.AutomationElement]::RootElement
        $walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
        $element = $focused
        while ($true) {
            $parent = $walker.GetParent($element)
            if ($null -eq $parent -or $parent.Equals($root)) { break }
            $element = $parent
        }
        return [pscustomobject]@{ Element = $element; ProcessId = $processId }
    }

    function Get-NodeName($element) {
        return [string]$element.Current.Name
    }

    function Get-NodeRole($element) {
        return [string]$element.Current.ControlType.ProgrammaticName
    }

    function Resolve-Target($target) {
        $current = Get-FocusedWindow
        if ($current.ProcessId -ne [int]$target.pid) { throw 'The foreground app changed; observe again.' }
        $element = $current.Element
        $walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
        foreach ($index in $target.path) {
            $child = $walker.GetFirstChild($element)
            for ($i = 0; $i -lt [int]$index -and $null -ne $child; $i++) {
                $child = $walker.GetNextSibling($child)
            }
            if ($null -eq $child) { throw 'The UI tree changed; observe again.' }
            $element = $child
        }
        if ((Get-NodeRole $element) -ne [string]$target.role -or (Get-NodeName $element) -ne [string]$target.name) {
            throw 'The target element changed; observe again.'
        }
        return $element
    }

    if ($req.op -eq 'screenshot') {
        $bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
        $bitmap = New-Object System.Drawing.Bitmap($bounds.Width, $bounds.Height)
        $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
        try {
            $graphics.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
            $bitmap.Save([string]$req.file_path, [System.Drawing.Imaging.ImageFormat]::Png)
        } finally {
            $graphics.Dispose()
            $bitmap.Dispose()
        }
        @{ screen_x = $bounds.X; screen_y = $bounds.Y; screen_width = $bounds.Width; screen_height = $bounds.Height } | ConvertTo-Json -Compress
        exit 0
    }

    if ($req.op -eq 'observe') {
        $current = Get-FocusedWindow
        $walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
        $queue = New-Object System.Collections.Queue
        $queue.Enqueue([pscustomobject]@{ Element = $current.Element; Path = @(); Depth = 0 })
        $nodes = New-Object 'System.Collections.Generic.List[object]'
        while ($queue.Count -gt 0 -and $nodes.Count -lt [int]$req.max_nodes) {
            $item = $queue.Dequeue()
            try {
                $nodes.Add(@{ pid = $current.ProcessId; path = @($item.Path); role = (Get-NodeRole $item.Element);
                    name = (Get-NodeName $item.Element); enabled = [bool]$item.Element.Current.IsEnabled })
                if ($item.Depth -ge 7) { continue }
                $child = $walker.GetFirstChild($item.Element)
                $index = 0
                while ($null -ne $child -and ($queue.Count + $nodes.Count) -lt ([int]$req.max_nodes * 3)) {
                    $queue.Enqueue([pscustomobject]@{ Element = $child; Path = @($item.Path) + @($index); Depth = $item.Depth + 1 })
                    $child = $walker.GetNextSibling($child)
                    $index++
                }
            } catch [System.Windows.Automation.ElementNotAvailableException] {
                continue
            }
        }
        @{ app = [string]$current.Element.Current.ProcessId; window = (Get-NodeName $current.Element);
            nodes = @($nodes.ToArray()) } | ConvertTo-Json -Depth 12 -Compress
        exit 0
    }

    if ($req.op -eq 'click' -and $null -ne $req.target) {
        $element = Resolve-Target $req.target
        $pattern = $null
        if ($element.TryGetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern, [ref]$pattern)) {
            ([System.Windows.Automation.InvokePattern]$pattern).Invoke()
        } elseif ($element.TryGetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern, [ref]$pattern)) {
            ([System.Windows.Automation.TogglePattern]$pattern).Toggle()
        } elseif ($element.TryGetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern, [ref]$pattern)) {
            ([System.Windows.Automation.SelectionItemPattern]$pattern).Select()
        } else {
            throw 'The element has no supported UI Automation action; use a screenshot and coordinates.'
        }
        @{ message = 'UI Automation action dispatched' } | ConvertTo-Json -Compress
        exit 0
    }

    if ($req.op -eq 'click') {
        $moved = $false
        for ($attempt = 0; $attempt -lt 3 -and -not $moved; $attempt++) {
            $moved = [SlimeBotDesktopInput]::SetCursorPos([int]$req.x, [int]$req.y)
            if (-not $moved) { Start-Sleep -Milliseconds 50 }
        }
        if (-not $moved) { throw 'SetCursorPos failed; the interactive desktop may not be accepting input.' }
        [SlimeBotDesktopInput]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
        [SlimeBotDesktopInput]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
        @{ message = 'Mouse click dispatched' } | ConvertTo-Json -Compress
        exit 0
    }

    if ($req.op -eq 'type' -and $null -ne $req.target) {
        $element = Resolve-Target $req.target
        $pattern = $null
        if (-not $element.TryGetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern, [ref]$pattern)) {
            throw 'The text control does not support ValuePattern.'
        }
        ([System.Windows.Automation.ValuePattern]$pattern).SetValue([string]$req.text)
        @{ message = 'UI Automation value set' } | ConvertTo-Json -Compress
        exit 0
    }

    if ($req.op -eq 'type') {
        [SlimeBotDesktopInput]::TypeText([string]$req.text)
        @{ message = 'Text typed into focused control' } | ConvertTo-Json -Compress
        exit 0
    }

    $keys = @{ Enter = 0x0D; Escape = 0x1B; Tab = 0x09; Backspace = 0x08;
        Up = 0x26; Down = 0x28; Left = 0x25; Right = 0x27; PageUp = 0x21; PageDown = 0x22 }
    if ($req.op -eq 'key') {
        [SlimeBotDesktopInput]::PressKey([uint16]$keys[[string]$req.key])
        @{ message = 'Key pressed' } | ConvertTo-Json -Compress
        exit 0
    }
    if ($req.op -eq 'scroll') {
        $key = if ($req.direction -eq 'up') { [uint16]0x21 } else { [uint16]0x22 }
        for ($i = 0; $i -lt [int]$req.amount; $i++) { [SlimeBotDesktopInput]::PressKey($key) }
        @{ message = 'Page scrolled' } | ConvertTo-Json -Compress
        exit 0
    }
    throw 'Unsupported desktop operation.'
} catch {
    @{ error = $_.Exception.Message } | ConvertTo-Json -Compress
}
