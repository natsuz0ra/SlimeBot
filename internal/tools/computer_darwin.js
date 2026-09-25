// macOS desktop adapter. osascript's JXA bridge calls the system AX API directly.
ObjC.import('Cocoa');
ObjC.import('ApplicationServices');
ObjC.bindFunction('AXUIElementCopyAttributeValue', ['int', ['id', 'id', 'id *']]);
ObjC.bindFunction('AXUIElementPerformAction', ['int', ['id', 'id']]);
ObjC.bindFunction('AXUIElementSetAttributeValue', ['int', ['id', 'id', 'id']]);
ObjC.bindFunction('AXValueGetValue', ['bool', ['id', 'int', 'void *']]);
ObjC.bindFunction('malloc', ['void *', ['int']]);
ObjC.bindFunction('free', ['void', ['void *']]);
ObjC.bindFunction('CGEventKeyboardSetUnicodeString', ['void', ['void *', 'unsigned long', 'void *']]);

function attribute(element, name) {
  const value = Ref();
  if ($.AXUIElementCopyAttributeValue(element, $(name), value) !== 0) return null;
  return value[0];
}

function stringValue(value) {
  if (value === null || value === undefined) return '';
  try { return String(ObjC.unwrap(value)); } catch (_) { return ''; }
}

function children(element) {
  const value = attribute(element, 'AXChildren');
  if (value === null) return [];
  try { return ObjC.deepUnwrap(value); } catch (_) { return []; }
}

function role(element) { return stringValue(attribute(element, 'AXRole')); }

function name(element) {
  const title = stringValue(attribute(element, 'AXTitle'));
  if (title) return title;
  const desc = stringValue(attribute(element, 'AXDescription'));
  if (desc) return desc;
  if (role(element) === 'AXStaticText') return stringValue(attribute(element, 'AXValue'));
  return '';
}

function frontmost() {
  const app = $.NSWorkspace.sharedWorkspace.frontmostApplication;
  if (!app) throw new Error('No foreground desktop application is available.');
  const pid = Number(app.processIdentifier);
  const axApp = ObjC.castRefToObject($.AXUIElementCreateApplication(pid));
  const window = attribute(axApp, 'AXFocusedWindow');
  return {
    pid: pid,
    app: stringValue(app.localizedName),
    window: window,
    root: window || axApp
  };
}

function locate(target) {
  const current = frontmost();
  if (current.pid !== target.pid) throw new Error('The foreground app changed; observe again.');
  let element = current.root;
  for (const index of target.path) {
    const items = children(element);
    if (index < 0 || index >= items.length) throw new Error('The UI tree changed; observe again.');
    element = items[index];
  }
  if (role(element) !== target.role || name(element) !== (target.name || '')) {
    throw new Error('The target element changed; observe again.');
  }
  return element;
}

function geometry(element, name, kind) {
  const value = attribute(element, name);
  if (value === null) throw new Error('Element has no ' + name + '.');
  const buffer = $.malloc(16);
  if (!buffer) throw new Error('Could not allocate AX geometry buffer.');
  try {
    if (!Boolean($.AXValueGetValue(value, kind, buffer))) throw new Error('Could not decode ' + name + '.');
    const bytes = Uint8Array.from(Array.from({length: 16}, (_, i) => Number(buffer[i])));
    const data = new DataView(bytes.buffer);
    return [data.getFloat64(0, true), data.getFloat64(8, true)];
  } finally {
    $.free(buffer);
  }
}

function pressKeyCode(code) {
  const down = $.CGEventCreateKeyboardEvent(null, code, true);
  const up = $.CGEventCreateKeyboardEvent(null, code, false);
  if (!down || !up) throw new Error('Could not create macOS keyboard events.');
  $.CGEventPost($.kCGHIDEventTap, down);
  $.CGEventPost($.kCGHIDEventTap, up);
}

function typeUnicode(value) {
  const length = value.length;
  const buffer = $.malloc(length * 2);
  if (!buffer) throw new Error('Could not allocate Unicode input buffer.');
  try {
    for (let i = 0; i < length; i++) {
      const code = value.charCodeAt(i);
      buffer[i * 2] = code & 255;
      buffer[i * 2 + 1] = code >> 8;
    }
    const down = $.CGEventCreateKeyboardEvent(null, 0, true);
    const up = $.CGEventCreateKeyboardEvent(null, 0, false);
    if (!down || !up) throw new Error('Could not create macOS keyboard events.');
    $.CGEventKeyboardSetUnicodeString(down, length, buffer);
    $.CGEventPost($.kCGHIDEventTap, down);
    $.CGEventPost($.kCGHIDEventTap, up);
  } finally {
    $.free(buffer);
  }
}

function screenInfo() {
  const frame = $.NSScreen.mainScreen.frame;
  return {
    screen_x: Number(frame.origin.x),
    screen_y: Number(frame.origin.y),
    screen_width: Number(frame.size.width),
    screen_height: Number(frame.size.height)
  };
}

function quoteShell(value) { return "'" + value.replace(/'/g, "'\\''") + "'"; }

function run() {
  try {
    const raw = ObjC.unwrap($.NSProcessInfo.processInfo.environment.objectForKey('SLIMEBOT_COMPUTER_REQUEST'));
    const req = JSON.parse(raw);
    if (req.op === 'screenshot') {
      const current = Application.currentApplication();
      current.includeStandardAdditions = true;
      current.doShellScript('/usr/sbin/screencapture -x -D 1 -t png ' + quoteShell(req.file_path));
      return JSON.stringify(screenInfo());
    }
    if (req.op !== 'observe' && !Boolean($.AXIsProcessTrusted())) {
      throw new Error('macOS Accessibility permission is required for the app that runs SlimeBot.');
    }
    if (req.op === 'observe') {
      const current = frontmost();
      const queue = [{element: current.root, path: [], depth: 0}];
      const nodes = [];
      while (queue.length && nodes.length < req.max_nodes) {
        const item = queue.shift();
        const currentRole = role(item.element);
        if (currentRole) {
          const enabled = attribute(item.element, 'AXEnabled');
          nodes.push({pid: current.pid, path: item.path, role: currentRole,
            name: name(item.element), enabled: enabled === null ? true : Boolean(ObjC.unwrap(enabled))});
        }
        if (item.depth >= 7) continue;
        const items = children(item.element);
        for (let i = 0; i < items.length && queue.length + nodes.length < req.max_nodes * 3; i++) {
          queue.push({element: items[i], path: item.path.concat(i), depth: item.depth + 1});
        }
      }
      return JSON.stringify({app: current.app, window: current.window ? stringValue(attribute(current.window, 'AXTitle')) : '', nodes: nodes});
    }
    if (req.op === 'bounds' && req.target) {
      const target = locate(req.target);
      const position = geometry(target, 'AXPosition', 1);
      const size = geometry(target, 'AXSize', 2);
      return JSON.stringify({screen_x: position[0], screen_y: position[1], screen_width: size[0], screen_height: size[1]});
    }
    if (req.op === 'click' && req.target) {
      const target = locate(req.target);
      const code = $.AXUIElementPerformAction(target, $('AXPress'));
      if (code !== 0) throw new Error('The element does not support AXPress (' + code + '); use a screenshot and coordinates.');
      return JSON.stringify({message: 'Accessibility action AXPress dispatched'});
    }
    if (req.op === 'click') {
      const point = $.CGPointMake(req.x, req.y);
      const down = $.CGEventCreateMouseEvent(null, $.kCGEventLeftMouseDown, point, $.kCGMouseButtonLeft);
      const up = $.CGEventCreateMouseEvent(null, $.kCGEventLeftMouseUp, point, $.kCGMouseButtonLeft);
      if (!down || !up) throw new Error('Could not create macOS mouse events.');
      $.CGEventPost($.kCGHIDEventTap, down);
      $.CGEventPost($.kCGHIDEventTap, up);
      return JSON.stringify({message: 'Mouse click dispatched'});
    }
    if (req.op === 'type' && req.target) {
      const target = locate(req.target);
      const targetRole = role(target);
      if (!['AXTextField', 'AXTextArea', 'AXComboBox', 'AXSearchField'].includes(targetRole)) {
        throw new Error('Target is not an editable text control.');
      }
      const code = $.AXUIElementSetAttributeValue(target, $('AXValue'), $.NSString.stringWithString(req.text));
      if (code !== 0) throw new Error('The text control does not support setting AXValue (' + code + ').');
      return JSON.stringify({message: 'Accessibility value set'});
    }
    if (req.op === 'type') {
      typeUnicode(req.text);
      return JSON.stringify({message: 'Text typed into focused control'});
    }
    const keys = {Enter: 36, Escape: 53, Tab: 48, Backspace: 51, Up: 126, Down: 125,
      Left: 123, Right: 124, PageUp: 116, PageDown: 121};
    if (req.op === 'key') {
      pressKeyCode(keys[req.key]);
      return JSON.stringify({message: 'Key pressed'});
    }
    if (req.op === 'scroll') {
      for (let i = 0; i < req.amount; i++) pressKeyCode(req.direction === 'up' ? 116 : 121);
      return JSON.stringify({message: 'Page scrolled'});
    }
    throw new Error('Unsupported desktop operation.');
  } catch (error) {
    return JSON.stringify({error: String(error)});
  }
}
