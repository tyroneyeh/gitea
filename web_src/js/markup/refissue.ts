import {queryElems} from '../utils/dom.ts';
import {parseIssueHref} from '../utils.ts';
import {createApp} from 'vue';
import {createTippy, getAttachedTippyInstance} from '../modules/tippy.ts';
import type {Instance} from 'tippy.js';

export function initMarkupRefIssue(el: HTMLElement) {
  queryElems(el, '.ref-issue', (el) => {
    el.addEventListener('mouseenter', showMarkupRefIssuePopup);
    el.addEventListener('focus', showMarkupRefIssuePopup);
  });
}

function showMarkupRefIssuePopup(e: MouseEvent | FocusEvent) {
  const refIssue = e.currentTarget as HTMLElement;
  if (getAttachedTippyInstance(refIssue)) return;
  if (refIssue.classList.contains('ref-external-issue')) return;

  const issuePathInfo = parseIssueHref(refIssue.getAttribute('href')!);
  if (!issuePathInfo.ownerName) return;
  const fetchedMap = new WeakMap<Instance, boolean>();

  const el = document.createElement('div');
  const onShowAsync = async () => {
    if (fetchedMap.has(tippy)) return;
    const {default: ContextPopup} = await import(/* webpackChunkName: "ContextPopup" */ '../components/ContextPopup.vue');
    const view = createApp(ContextPopup, {
      // backend: GetIssueInfo
      loadIssueInfoUrl: `${window.config.appSubUrl}/${issuePathInfo.ownerName}/${issuePathInfo.repoName}/issues/${issuePathInfo.indexString}/info`,
    });
    view.mount(el);
    fetchedMap.set(tippy, true);
  };
  const tippy = createTippy(refIssue, {
    theme: 'default',
    content: el,
    trigger: 'mouseenter focus',
    placement: 'top-start',
    interactive: true,
    role: 'dialog',
    interactiveBorder: 5,
    // onHide() { return false }, // help to keep the popup and debug the layout
    onShow: () => { onShowAsync() },
  });
  tippy.show();
}
