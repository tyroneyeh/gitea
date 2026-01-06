import {isElemVisible, onInputDebounce, submitEventSubmitter, toggleElem} from '../utils/dom.ts';

const {appSubUrl} = window.config;
const reIssueSharpIndex = /^#(\d+)$/; // eg: "#123"
const reIssueOwnerRepoIndex = /([-.\w]+)\/([-.\w]+)#(\d+)$/;  // eg: "{owner}/{repo}#{index}"

// if the searchText can be parsed to an "issue goto link", return the link, otherwise return empty string
export function parseIssueListQuickGotoLink(repoLink: string, searchText: string) {
  searchText = searchText.trim();
  let targetUrl = '';
  const [_, owner, repo, index] = reIssueOwnerRepoIndex.exec(searchText) || [];
  // try to parse it in current repo
  if (reIssueSharpIndex.test(searchText)) {
    targetUrl = `${repoLink}/issues/${searchText.substring(1)}`;
  } else if (owner) {
    // try to parse it for a global search (eg: "owner/repo#123")
    targetUrl = `${appSubUrl}/${owner}/${repo}/issues/${index}`;
  }
  return targetUrl;
}

export function initCommonIssueListQuickGoto() {
  const gotos = document.querySelectorAll<HTMLElement>('#issue-list-quick-goto');
  if (!gotos.length) return;

  for (const goto of gotos) {
    const form = goto.closest('form')!;
    const input = form.querySelector<HTMLInputElement>('input[name=q]')!;
    const repoLink = goto.getAttribute('data-repo-link')!;

    const onInput = () => {
      const searchText = input.value;
      // try to check whether the parsed goto link is valid
      let targetUrl;
      if (repoLink.length && !Number.isNaN(Number(searchText))) {
        // also support issue index only (eg: "123")
        targetUrl = `${repoLink}/issues/${Number(searchText)}`;
      } else {
        targetUrl = parseIssueListQuickGotoLink(repoLink, searchText);
      }
      toggleElem(goto, Boolean(targetUrl));
      goto.setAttribute('data-issue-goto-link', targetUrl);
    };

    input.addEventListener('input', onInputDebounce(onInput));
  }
}
