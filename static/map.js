// Dependency map: draws a curve for every dependency and highlights a
// node's chain (what it needs and what needs it) on hover.
(function () {
  var map = document.getElementById('depmap');
  if (!map) return;
  var svg = map.querySelector('.map-lines');
  var nodes = {};
  map.querySelectorAll('.map-node').forEach(function (el) { nodes[el.dataset.key] = el; });
  var edges = [];
  Object.keys(nodes).forEach(function (k) {
    (nodes[k].dataset.deps || '').split(' ').forEach(function (d) {
      if (d && nodes[d]) edges.push({ from: k, to: d });
    });
  });

  function draw() {
    var box = map.getBoundingClientRect();
    svg.setAttribute('width', map.scrollWidth);
    svg.setAttribute('height', map.scrollHeight);
    var out = '';
    edges.forEach(function (e, i) {
      var a = nodes[e.from].getBoundingClientRect(), b = nodes[e.to].getBoundingClientRect();
      var x1 = a.right - box.left, y1 = a.top + a.height / 2 - box.top;
      var x2 = b.left - box.left, y2 = b.top + b.height / 2 - box.top;
      if (x2 < x1) { // same or earlier column: connect left sides
        x1 = a.left - box.left; x2 = b.left - box.left;
      }
      var dx = Math.max(30, Math.abs(x2 - x1) / 2);
      out += '<path data-i="' + i + '" d="M' + x1 + ',' + y1 + ' C' + (x1 + dx) + ',' + y1 + ' ' + (x2 - dx) + ',' + y2 + ' ' + x2 + ',' + y2 + '"/>';
    });
    svg.innerHTML = out;
  }

  function chain(start, dir) { // dir: 'down' = what start needs, 'up' = what needs start
    var seen = {}, queue = [start];
    seen[start] = true;
    while (queue.length) {
      var cur = queue.shift();
      edges.forEach(function (e) {
        var next = dir === 'down' ? (e.from === cur ? e.to : null) : (e.to === cur ? e.from : null);
        if (next && !seen[next]) { seen[next] = true; queue.push(next); }
      });
    }
    return seen;
  }

  function highlight(key) {
    var on = key ? Object.assign(chain(key, 'down'), chain(key, 'up')) : null;
    map.classList.toggle('hovering', !!on);
    Object.keys(nodes).forEach(function (k) { nodes[k].classList.toggle('lit', !!(on && on[k])); });
    svg.querySelectorAll('path').forEach(function (p) {
      var e = edges[+p.dataset.i];
      p.classList.toggle('lit', !!(on && on[e.from] && on[e.to]));
    });
  }

  Object.keys(nodes).forEach(function (k) {
    nodes[k].addEventListener('mouseenter', function () { highlight(k); });
    nodes[k].addEventListener('mouseleave', function () { highlight(null); });
  });
  draw();
  window.addEventListener('resize', draw);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(draw);
})();
