// Injected at the top of <head> before any page script runs. Records script
// errors and console.error calls so the measure pass can report them.
(function () {
  window.__sgErrors = [];
  window.addEventListener('error', function (e) {
    var where = e.filename ? ' (' + e.filename.split('/').pop() + ':' + e.lineno + ')' : '';
    if (e.target && e.target !== window && e.target.tagName) {
      window.__sgErrors.push('failed to load <' + e.target.tagName.toLowerCase() + '> ' + (e.target.src || e.target.href || ''));
      return;
    }
    window.__sgErrors.push(String(e.message || 'error') + where);
  }, true);
  window.addEventListener('unhandledrejection', function (e) {
    window.__sgErrors.push('unhandled rejection: ' + String(e.reason));
  });
  var original = console.error;
  console.error = function () {
    try { window.__sgErrors.push('console.error: ' + Array.prototype.join.call(arguments, ' ')); } catch (_) {}
    return original.apply(console, arguments);
  };
})();
