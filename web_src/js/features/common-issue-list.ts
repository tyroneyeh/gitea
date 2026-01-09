import {isElemVisible, toggleElem} from '../utils/dom.ts';

const {appSubUrl} = window.config;
const reIssueIndex = /^(\d+)$/; // eg: "123"
const reIssueSharpIndex = /^#(\d+)$/; // eg: "#123"
const reIssueOwnerRepoIndex = /([-.\w]+)\/([-.\w]+)#(\d+)$/;  // eg: "{owner}/{repo}#{index}"

// if the searchText can be parsed to an "issue goto link", return the link, otherwise return empty string
export function parseIssueListQuickGotoLink(repoLink: string, searchText: string) {
  searchText = searchText.trim();
  let targetUrl = '';
  const [_, owner, repo, index] = reIssueOwnerRepoIndex.exec(searchText) || [];
  // try to parse it in current repo
  if (reIssueIndex.test(searchText)) {
    targetUrl = `${repoLink}/issues/${searchText}`;
  } else if (reIssueSharpIndex.test(searchText)) {
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
  let isHash = false;

  const quickGoto = (goto: HTMLElement) => {
    const link = goto.getAttribute('data-issue-goto-link');
    if (link) {
      window.location.href = link;
    }
  };

  for (const goto of gotos) {
    const form = goto.closest('form')!;
    const input = form.querySelector<HTMLInputElement>('input[name=q]')!;
    const repoLink = goto.getAttribute('data-repo-link')!;

    form.addEventListener('submit', (e) => {
      // if there is no goto button, or the form is submitted by non-quick-goto elements, submit the form directly
      if (!isElemVisible(goto) || !isHash) return;

      // if there is a goto button, use its link
      e.preventDefault();
      quickGoto(goto);
    });

    goto.addEventListener('click', () => {
      quickGoto(goto);
    });

    const onInput = () => {
      const searchText = input.value;
      // try to check whether the parsed goto link is valid
      let targetUrl = parseIssueListQuickGotoLink(repoLink, searchText);
      if (targetUrl) {
        if (['/', '/issues', '/pulls'].includes(location.pathname)) {
          targetUrl = '';
        } else {
          isHash = searchText.startsWith('#');
        }
        toggleElem(goto, Boolean(targetUrl));
      }
      goto.setAttribute('data-issue-goto-link', targetUrl);
    };

    input.addEventListener('input', onInput);
  }
}
