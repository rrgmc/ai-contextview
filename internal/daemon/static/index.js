// ai-specview index page: live session list, notification permission, editor setting.
(function () {
  "use strict";
  const store = {
    get(k, d) { try { const v = localStorage.getItem("ai-specview." + k); return v === null ? d : v; } catch (e) { return d; } },
    set(k, v) { try { localStorage.setItem("ai-specview." + k, v); } catch (e) { /* private mode */ } },
  };
  const editor = document.getElementById("editor");
  editor.value = store.get("editor", "vscode");
  editor.addEventListener("change", () => store.set("editor", editor.value));

  const btn = document.getElementById("notify");
  const state = document.getElementById("notify-state");
  function showState() {
    if (!("Notification" in window)) { btn.disabled = true; state.textContent = "not supported by this browser"; return; }
    state.textContent = Notification.permission === "granted" ? "enabled" : Notification.permission === "denied" ? "blocked in browser settings" : "";
    btn.hidden = Notification.permission === "granted";
  }
  btn.addEventListener("click", () => Notification.requestPermission().then(showState));
  showState();

  document.querySelectorAll(".close-ws").forEach((b) => b.addEventListener("click", async () => {
    await fetch("/w/" + encodeURIComponent(b.dataset.ws) + "/close", { method: "POST" });
    location.reload();
  }));

  let timer = null;
  const es = new EventSource("/events");
  es.addEventListener("changed", () => {
    clearTimeout(timer);
    timer = setTimeout(() => location.reload(), 300);
  });
})();
