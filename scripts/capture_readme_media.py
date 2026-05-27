#!/usr/bin/env python3
"""Capture reproducible frontend media assets for README branding."""

from __future__ import annotations

import argparse
import re
import tempfile
import time
from pathlib import Path
from typing import Callable

from PIL import Image
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import sync_playwright


def _wait_for_text(page, text: str, timeout_ms: int = 15000) -> None:
    page.wait_for_selector(f"text={text}", timeout=timeout_ms)


def _click_tab(page, name: str | re.Pattern[str]) -> None:
    """Click a tab by accessible name (role=tab in current UI, button in older builds)."""
    tab = page.get_by_role("tab", name=name)
    if tab.count() > 0:
        tab.first.click()
        return
    page.get_by_role("button", name=name).first.click()


def _ensure_demo_cursor(page) -> None:
    page.evaluate(
        """
        () => {
          if (window.__orlojDemoCursorReady) return;

          const style = document.createElement('style');
          style.textContent = `
            #orloj-demo-cursor {
              position: fixed;
              left: 0;
              top: 0;
              width: 26px;
              height: 36px;
              pointer-events: none;
              z-index: 2147483647;
              transform: translate(18px, 18px);
              transition: transform 320ms ease;
            }
            #orloj-demo-cursor svg {
              width: 100%;
              height: 100%;
              filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.6));
            }
            #orloj-demo-cursor-ring {
              position: fixed;
              width: 12px;
              height: 12px;
              border-radius: 999px;
              border: 2px solid rgba(255, 255, 255, 0.95);
              box-shadow: 0 0 0 1px rgba(10, 10, 10, 0.9), 0 0 0 4px rgba(59, 130, 246, 0.35);
              pointer-events: none;
              z-index: 2147483646;
              opacity: 0;
              transform: translate(-9999px, -9999px) scale(0.6);
              transition: transform 180ms ease, opacity 180ms ease;
            }
          `;
          document.head.appendChild(style);

          const cursor = document.createElement('div');
          cursor.id = 'orloj-demo-cursor';
          cursor.innerHTML = `
            <svg viewBox="0 0 18 24" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
              <path d="M2 1 L2 20 L7 15 L10.5 23 L13.5 21.8 L10 14.2 L16.8 14.2 Z"
                    fill="white" stroke="#0f172a" stroke-width="1.2" />
            </svg>
          `;
          document.body.appendChild(cursor);

          const ring = document.createElement('div');
          ring.id = 'orloj-demo-cursor-ring';
          document.body.appendChild(ring);

          window.__orlojDemoCursorMove = (x, y) => {
            cursor.style.transform = `translate(${Math.round(x)}px, ${Math.round(y)}px)`;
          };
          window.__orlojDemoCursorPulse = (x, y) => {
            ring.style.opacity = '0.95';
            ring.style.transform = `translate(${Math.round(x + 8)}px, ${Math.round(y + 8)}px) scale(1)`;
            setTimeout(() => {
              ring.style.opacity = '0';
              ring.style.transform = `translate(${Math.round(x + 8)}px, ${Math.round(y + 8)}px) scale(1.65)`;
            }, 20);
          };
          window.__orlojDemoCursorReady = true;
        }
        """
    )


def _move_demo_cursor(page, x: float, y: float, settle_ms: int = 340) -> None:
    _ensure_demo_cursor(page)
    page.evaluate("([x,y]) => window.__orlojDemoCursorMove(x,y)", [x, y])
    page.wait_for_timeout(settle_ms)


def _pulse_demo_cursor(page, x: float, y: float) -> None:
    _ensure_demo_cursor(page)
    page.evaluate("([x,y]) => window.__orlojDemoCursorPulse(x,y)", [x, y])
    page.wait_for_timeout(130)


def _locator_center(locator) -> tuple[float, float]:
    box = locator.bounding_box()
    if box is None:
        raise RuntimeError("Could not resolve locator bounding box for demo cursor.")
    return box["x"] + box["width"] * 0.5, box["y"] + box["height"] * 0.5


def _demo_click(
    page,
    locator,
    settle_before_click_ms: int = 260,
    after_click_ms: int = 460,
    recorder: Callable[[int], None] | None = None,
) -> None:
    target = locator.first
    target.scroll_into_view_if_needed(timeout=10000)
    x, y = _locator_center(target)
    _move_demo_cursor(page, x, y)
    if recorder:
        recorder(settle_before_click_ms)
    else:
        page.wait_for_timeout(settle_before_click_ms)
    _pulse_demo_cursor(page, x, y)
    target.click()
    if recorder:
        recorder(after_click_ms)
    else:
        page.wait_for_timeout(after_click_ms)


def _capture_dashboard(page, base_url: str, namespace: str, out_dir: Path) -> None:
    page.goto(f"{base_url}/", wait_until="domcontentloaded")
    _wait_for_text(page, "Dashboard")
    _set_namespace(page, namespace)
    _move_demo_cursor(page, 54, 84, settle_ms=220)
    page.wait_for_timeout(1200)
    page.screenshot(path=str(out_dir / "dashboard-overview.png"), full_page=False)


def _set_namespace(page, namespace: str) -> None:
    ns_input = page.get_by_label("Namespace").first
    ns_input.click()
    ns_input.fill(namespace)
    ns_input.press("Enter")
    page.wait_for_timeout(500)


def _open_task_detail_via_ui(page, task_name: str) -> None:
    page.get_by_role("link", name="Tasks").click()
    _wait_for_text(page, "Tasks")
    page.wait_for_selector("tbody tr", timeout=15000)
    row = page.locator("tbody tr", has_text=task_name).first
    if row.count() == 0:
        row = page.locator("tbody tr.table-row--clickable").first
    row.click()
    page.wait_for_selector(".tab-bar", timeout=15000)


def _open_system_topology_via_ui(page, system_name: str) -> None:
    page.get_by_role("link", name="Agent Systems").click()
    _wait_for_text(page, "Agent Systems")
    card = page.locator(".resource-card", has_text=system_name).first
    if card.count() > 0:
        card.click()
    else:
        row = page.locator("tbody tr", has_text=system_name).first
        if row.count() > 0:
            row.click()
        else:
            page.get_by_text(system_name).first.click()
    _wait_for_text(page, system_name)
    tree_tab = page.get_by_role("tab", name="Resource Tree")
    if tree_tab.count() == 0:
        tree_tab = page.get_by_role("button", name="Resource Tree")
    if tree_tab.count() > 0:
        tree_tab.first.click()
    page.wait_for_selector(".graph-view", timeout=15000)


def _capture_system_topology(page, base_url: str, namespace: str, system_name: str, out_dir: Path) -> None:
    page.goto(f"{base_url}/", wait_until="domcontentloaded")
    _wait_for_text(page, "Dashboard")
    _set_namespace(page, namespace)
    _open_system_topology_via_ui(page, system_name)
    page.wait_for_timeout(1000)
    page.screenshot(path=str(out_dir / "system-topology.png"), full_page=False)


def _capture_task_graph(page, base_url: str, namespace: str, task_name: str, out_dir: Path) -> None:
    _capture_task_detail_views(page, base_url, namespace, task_name, out_dir)


def _capture_task_trace_logs(page, base_url: str, namespace: str, task_name: str, out_dir: Path) -> None:
    # Kept for compatibility; task trace capture is combined with graph capture.
    pass


def _capture_task_detail_views(page, base_url: str, namespace: str, task_name: str, out_dir: Path) -> None:
    page.goto(f"{base_url}/", wait_until="domcontentloaded", timeout=60000)
    _wait_for_text(page, "Dashboard")
    _set_namespace(page, namespace)
    _open_task_detail_via_ui(page, task_name)
    try:
        page.wait_for_selector(".tab-bar", timeout=15000)
    except PlaywrightTimeoutError:
        page.screenshot(path=str(out_dir / "_debug-task-page.png"), full_page=True)
        (out_dir / "_debug-task-page.html").write_text(page.content(), encoding="utf-8")
        raise
    _click_tab(page, "Graph")
    page.wait_for_timeout(1600)
    page.screenshot(path=str(out_dir / "task-detail-graph.png"), full_page=False)

    trace_tab = page.get_by_role("tab", name=re.compile(r"^Trace"))
    if trace_tab.count() == 0:
        trace_tab = page.get_by_role("button", name=re.compile(r"^Trace"))
    if trace_tab.count() > 0:
        trace_tab.first.click()
    else:
        page.locator(".tab-bar__tab", has_text="Trace").first.click()
    try:
        page.wait_for_selector(".trace-view", timeout=10000)
    except PlaywrightTimeoutError:
        page.wait_for_timeout(1200)
    page.screenshot(path=str(out_dir / "task-trace-logs.png"), full_page=False)


def _read_task_phase(page) -> str:
    try:
        return page.locator(".page__header .badge").first.inner_text(timeout=1000).strip()
    except PlaywrightTimeoutError:
        return "Unknown"


def _make_gif(images: list[Path], output_path: Path, durations_ms: list[int] | None = None) -> None:
    processed = []
    durations: list[int] = []
    for idx, frame_path in enumerate(images):
        with Image.open(frame_path) as frame:
            rgb = frame.convert("RGB").resize((900, 506), Image.Resampling.LANCZOS)
            palette = rgb.convert("P", palette=Image.ADAPTIVE, colors=96)
            processed.append(palette)
            if durations_ms and idx < len(durations_ms):
                durations.append(max(80, int(durations_ms[idx])))
            else:
                durations.append(160 if idx in (0, len(images) - 1) else 140)

    if not processed:
        raise RuntimeError("No frames captured for GIF generation.")

    processed[0].save(
        output_path,
        save_all=True,
        append_images=processed[1:],
        loop=0,
        duration=durations,
        optimize=True,
    )


def _capture_lifecycle_gif(
    page,
    base_url: str,
    namespace: str,
    task_name: str,
    system_name: str,
    out_dir: Path,
) -> None:
    with tempfile.TemporaryDirectory(prefix="orloj-readme-gif-") as tmp:
        tmp_dir = Path(tmp)
        frames: list[Path] = []
        frame_durations: list[int] = []

        def record_for(total_ms: int, frame_ms: int = 140) -> None:
            if total_ms <= 0:
                return
            remaining = total_ms
            while remaining > 0:
                step = min(frame_ms, remaining)
                frame_path = tmp_dir / f"frame-{len(frames):03d}.png"
                page.screenshot(path=str(frame_path), full_page=False)
                frames.append(frame_path)
                frame_durations.append(step)
                page.wait_for_timeout(step)
                remaining -= step

        # 1) Dashboard
        page.goto(f"{base_url}/", wait_until="domcontentloaded", timeout=60000)
        _wait_for_text(page, "Dashboard")
        _set_namespace(page, namespace)
        _move_demo_cursor(page, 54, 84, settle_ms=220)
        record_for(1200)

        # 2) Agent Systems list
        _demo_click(
            page,
            page.get_by_role("link", name="Agent Systems"),
            recorder=lambda ms: record_for(ms),
        )
        _wait_for_text(page, "Agent Systems")
        record_for(950)

        # 3) Open the pipeline system topology
        system_card = page.locator(".resource-card", has_text=system_name).first
        if system_card.count() > 0:
            _demo_click(page, system_card, recorder=lambda ms: record_for(ms))
        else:
            _demo_click(page, page.get_by_text(system_name), recorder=lambda ms: record_for(ms))
        _wait_for_text(page, system_name)
        resource_tree = page.get_by_role("tab", name="Resource Tree")
        if resource_tree.count() == 0:
            resource_tree = page.get_by_role("button", name="Resource Tree")
        if resource_tree.count() > 0:
            _demo_click(
                page,
                resource_tree.first,
                recorder=lambda ms: record_for(ms),
            )
        record_for(1100)

        # 4) Click task node in topology -> open task detail
        task_node = page.locator(".react-flow__node", has_text=task_name).first
        if task_node.count() == 0:
            task_node = page.locator(".react-flow__node", has_text="task").first
        _demo_click(
            page,
            task_node,
            settle_before_click_ms=320,
            after_click_ms=580,
            recorder=lambda ms: record_for(ms),
        )
        page.wait_for_selector(".tab-bar", timeout=15000)
        record_for(1000)

        # 5) Trace tab
        trace_tab = page.get_by_role("tab", name=re.compile(r"^Trace"))
        if trace_tab.count() == 0:
            trace_tab = page.get_by_role("button", name=re.compile(r"^Trace"))
        if trace_tab.count() > 0:
            _demo_click(page, trace_tab.first, recorder=lambda ms: record_for(ms))
        else:
            _demo_click(page, page.locator(".tab-bar__tab", has_text="Trace").first, recorder=lambda ms: record_for(ms))
        record_for(950)

        # 6) YAML tab
        yaml_tab = page.get_by_role("tab", name="YAML")
        if yaml_tab.count() == 0:
            yaml_tab = page.get_by_role("button", name="YAML")
        _demo_click(page, yaml_tab.first, recorder=lambda ms: record_for(ms))
        record_for(1350)

        _make_gif(frames, out_dir / "task-run-lifecycle.gif", frame_durations)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--out-dir", default="docs/public/readme")
    parser.add_argument("--reference-task", default="rr-real-pipeline-task")
    parser.add_argument("--gif-task")
    parser.add_argument("--system-name", default="rr-real-pipeline-system")
    parser.add_argument("--namespace", default="rr-real-pipeline")
    args = parser.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(
            viewport={"width": 1728, "height": 1080},
            color_scheme="light",
            locale="en-US",
        )
        page = context.new_page()

        _capture_dashboard(page, args.base_url, args.namespace, out_dir)
        _capture_system_topology(page, args.base_url, args.namespace, args.system_name, out_dir)

        task_for_static = args.reference_task
        try:
            _capture_task_detail_views(page, args.base_url, args.namespace, task_for_static, out_dir)
        except PlaywrightTimeoutError:
            if not args.gif_task:
                raise
            # Fallback when local namespace/profile differs from expected defaults.
            _capture_task_detail_views(page, args.base_url, args.namespace, args.gif_task, out_dir)

        gif_task = args.gif_task or args.reference_task
        _capture_lifecycle_gif(
            page,
            args.base_url,
            args.namespace,
            gif_task,
            args.system_name,
            out_dir,
        )

        context.close()
        browser.close()

    # Small pause avoids race on some filesystems before consumers read the assets.
    time.sleep(0.1)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
