"""Linux desktop adapter: AT-SPI2 for semantic actions; session tools for pixels."""

import json
import os
import shutil
import subprocess
import sys


def run_command(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=12)
    if result.returncode != 0:
        raise RuntimeError((result.stderr or result.stdout or "command failed").strip())


def legacy_ydotool():
    # Ubuntu 24.04 ships 0.1.8, whose key and click syntax predates ydotool 1.x.
    result = subprocess.run(["ydotool", "help"], capture_output=True, text=True, timeout=3)
    return "recorder" in (result.stdout + result.stderr).lower()


def session_type():
    if os.environ.get("XDG_SESSION_TYPE", "").lower() == "wayland" or os.environ.get("WAYLAND_DISPLAY"):
        return "wayland"
    if os.environ.get("DISPLAY"):
        return "x11"
    raise RuntimeError("No graphical X11 or Wayland session is available to the SlimeBot process.")


def screenshot(path):
    session = session_type()
    candidates = []
    if session == "wayland" and shutil.which("grim"):
        candidates.append(["grim", path])
    spectacle = ["spectacle", "-b", "-n", "-f", "-o", path]
    kde_session = "KDE" in os.environ.get("XDG_CURRENT_DESKTOP", "").upper().split(":")
    if session == "wayland" and kde_session and shutil.which("spectacle"):
        candidates.append(spectacle)
    if shutil.which("gnome-screenshot"):
        candidates.append(["gnome-screenshot", "-f", path])
    if session == "wayland" and not kde_session and shutil.which("spectacle"):
        candidates.append(spectacle)
    if session == "x11" and shutil.which("scrot"):
        candidates.append(["scrot", "-o", path])
    if session == "x11" and shutil.which("import"):
        candidates.append(["import", "-window", "root", path])
    errors = []
    for command in candidates:
        try:
            run_command(command)
            if os.path.getsize(path) > 0:
                return {"message": "Screenshot captured"}
            errors.append(command[0] + " produced an empty screenshot")
        except (RuntimeError, OSError, subprocess.TimeoutExpired) as error:
            errors.append(str(error))
    required = "grim, Spectacle, or gnome-screenshot" if session == "wayland" else "gnome-screenshot, scrot, or ImageMagick import"
    raise RuntimeError("Screenshot unavailable; install " + required + ". " + "; ".join(errors[:2]))


def accessibility():
    try:
        import pyatspi
    except ImportError as error:
        raise RuntimeError("AT-SPI2 Python bindings are missing; install python3-pyatspi.") from error
    return pyatspi


def active_window(pyatspi):
    desktop = pyatspi.Registry.getDesktop(0)
    for app in desktop:
        for window in app:
            try:
                if window.getState().contains(pyatspi.STATE_ACTIVE):
                    return app, window, app_process_id(app)
            except Exception:
                continue
    if session_type() == "x11" and shutil.which("xdotool"):
        try:
            focused = subprocess.run(["xdotool", "getwindowfocus"], capture_output=True, text=True, timeout=3)
            window_id = focused.stdout.strip()
            if focused.returncode == 0 and window_id:
                title = subprocess.run(["xdotool", "getwindowname", window_id], capture_output=True, text=True, timeout=3)
                pid = subprocess.run(["xdotool", "getwindowpid", window_id], capture_output=True, text=True, timeout=3)
                process_id = int(pid.stdout.strip()) if pid.returncode == 0 and pid.stdout.strip().isdigit() else 0
                if title.returncode == 0 and title.stdout.strip():
                    for app in desktop:
                        for window in app:
                            try:
                                app_pid = app_process_id(app)
                                if node_name(window) == title.stdout.strip() and (not process_id or not app_pid or process_id == app_pid):
                                    return app, window, app_pid
                            except Exception:
                                continue
        except (OSError, subprocess.TimeoutExpired):
            pass
    raise RuntimeError("AT-SPI2 did not report an active application window.")


def app_process_id(app):
    try:
        return int(app.get_process_id())
    except (AttributeError, TypeError, ValueError):
        return 0


def node_name(node):
    return str(node.name or "")


def node_role(node):
    return str(node.getRoleName() or "")


def locate(pyatspi, target):
    app, root, process_id = active_window(pyatspi)
    if process_id != int(target.get("pid", 0)):
        raise RuntimeError("The foreground app changed; observe again.")
    node = root
    for index in target["path"]:
        if index < 0 or index >= node.childCount:
            raise RuntimeError("The UI tree changed; observe again.")
        node = node[index]
    if node_role(node) != target["role"] or node_name(node) != target.get("name", ""):
        raise RuntimeError("The target element changed; observe again.")
    return node


def observe(pyatspi, max_nodes):
    app, root, process_id = active_window(pyatspi)
    queue = [(root, [], 0)]
    nodes = []
    while queue and len(nodes) < max_nodes:
        node, path, depth = queue.pop(0)
        try:
            nodes.append({"pid": process_id, "path": path, "role": node_role(node),
                          "name": node_name(node), "enabled": bool(node.getState().contains(pyatspi.STATE_ENABLED))})
            if depth >= 7:
                continue
            for index in range(min(node.childCount, max_nodes * 3)):
                if len(queue) + len(nodes) >= max_nodes * 3:
                    break
                queue.append((node[index], path + [index], depth + 1))
        except Exception:
            continue
    return {"app": node_name(app), "window": node_name(root), "nodes": nodes}


def bounds(pyatspi, target):
    node = locate(pyatspi, target)
    rect = node.queryComponent().getExtents(pyatspi.DESKTOP_COORDS)
    return {"screen_x": rect.x, "screen_y": rect.y,
            "screen_width": rect.width, "screen_height": rect.height}


def click_coordinates(req):
    if session_type() == "x11":
        if not shutil.which("xdotool"):
            raise RuntimeError("X11 coordinate clicks require xdotool.")
        run_command(["xdotool", "mousemove", str(req["x"]), str(req["y"]), "click", "1"])
    else:
        if not shutil.which("ydotool"):
            raise RuntimeError("Wayland coordinate clicks require ydotool and a running ydotoold daemon.")
        if legacy_ydotool():
            run_command(["ydotool", "mousemove", str(req["x"]), str(req["y"])])
            run_command(["ydotool", "click", "1"])
        else:
            run_command(["ydotool", "mousemove", "--absolute", str(req["x"]), str(req["y"])])
            run_command(["ydotool", "click", "0xC0"])
    return {"message": "Mouse click dispatched"}


KEY_CODES = {"Enter": 28, "Escape": 1, "Tab": 15, "Backspace": 14,
             "Up": 103, "Down": 108, "Left": 105, "Right": 106,
             "PageUp": 104, "PageDown": 109}
X11_KEY_NAMES = {"Enter": "Return", "Escape": "Escape", "Tab": "Tab", "Backspace": "BackSpace",
                 "Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right",
                 "PageUp": "Prior", "PageDown": "Next"}
LEGACY_YDOTOOL_KEY_NAMES = {"Enter": "Enter", "Escape": "Escape", "Tab": "Tab", "Backspace": "Backspace",
                            "Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right",
                            "PageUp": "PageUp", "PageDown": "PageDown"}


def keypress(key):
    session = session_type()
    if session == "x11":
        if not shutil.which("xdotool"):
            raise RuntimeError("X11 keyboard input requires xdotool.")
        run_command(["xdotool", "key", "--clearmodifiers", X11_KEY_NAMES[key]])
    else:
        if not shutil.which("ydotool"):
            raise RuntimeError("Wayland keyboard input requires ydotool and a running ydotoold daemon.")
        if legacy_ydotool():
            run_command(["ydotool", "key", LEGACY_YDOTOOL_KEY_NAMES[key]])
        else:
            code = KEY_CODES[key]
            run_command(["ydotool", "key", f"{code}:1", f"{code}:0"])


def type_focused(text):
    session = session_type()
    if session == "x11":
        if not shutil.which("xdotool"):
            raise RuntimeError("X11 text input requires xdotool.")
        run_command(["xdotool", "type", "--clearmodifiers", "--", text])
    else:
        if not shutil.which("ydotool"):
            raise RuntimeError("Wayland text input requires ydotool and a running ydotoold daemon.")
        run_command(["ydotool", "type", "--", text])


def main(req):
    op = req["op"]
    if op == "screenshot":
        return screenshot(req["file_path"])
    if op == "click" and not req.get("target"):
        return click_coordinates(req)
    if op == "type" and not req.get("target"):
        type_focused(req["text"])
        return {"message": "Text typed into focused control"}
    if op == "key":
        keypress(req["key"])
        return {"message": "Key pressed"}
    if op == "scroll":
        key = "PageUp" if req["direction"] == "up" else "PageDown"
        for _ in range(req["amount"]):
            keypress(key)
        return {"message": "Page scrolled"}
    pyatspi = accessibility()
    if op == "observe":
        return observe(pyatspi, int(req["max_nodes"]))
    if op == "bounds":
        return bounds(pyatspi, req["target"])
    if op == "read":
        node = locate(pyatspi, req["target"])
        text = node.queryText()
        return {"message": text.getText(0, text.characterCount)}
    if op == "click" and req.get("target"):
        node = locate(pyatspi, req["target"])
        action = node.queryAction()
        if action.nActions < 1:
            raise RuntimeError("Element has no AT-SPI action; use screenshot coordinates.")
        chosen = next((i for i in range(action.nActions) if action.getName(i).lower() in ("click", "press", "activate")), 0)
        if not action.doAction(chosen):
            raise RuntimeError("AT-SPI action failed; observe again.")
        return {"message": "AT-SPI action dispatched"}
    if op == "type" and req.get("target"):
        node = locate(pyatspi, req["target"])
        if not node.queryEditableText().setTextContents(req["text"]):
            raise RuntimeError("AT-SPI editable text operation failed.")
        return {"message": "AT-SPI text value set"}
    raise RuntimeError("Unsupported desktop operation.")


try:
    print(json.dumps(main(json.loads(os.environ["SLIMEBOT_COMPUTER_REQUEST"])), ensure_ascii=False))
except Exception as error:
    print(json.dumps({"error": str(error)}, ensure_ascii=False))
