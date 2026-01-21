import {imageInfo} from '../../utils/image.ts';
import {replaceTextareaSelection, triggerEditorContentChanged} from './EditorMarkdown.ts';
import {
  DropzoneCustomEventRemovedFile,
  DropzoneCustomEventUploadDone,
  generateMarkdownLinkForAttachment,
} from '../dropzone.ts';
import {subscribe} from '@github/paste-markdown';
import type CodeMirror from 'codemirror';
import type EasyMDE from 'easymde';
import type {DropzoneFile} from 'dropzone';
import {isImageFile, isVideoFile, isCompressedFile, isPDFFile, compressFileToZip} from '../../utils.ts';

let uploadIdCounter = 0;

export const EventUploadStateChanged = 'ce-upload-state-changed';

export function triggerUploadStateChanged(target: HTMLElement) {
  target.dispatchEvent(new CustomEvent(EventUploadStateChanged, {bubbles: true}));
}

function uploadFile(dropzoneEl: HTMLElement, file: File, removePlaceholder: () => void) {
  return new Promise((resolve) => {
    const curUploadId = uploadIdCounter++;
    (file as any)._giteaUploadId = curUploadId;
    const dropzoneInst = dropzoneEl.dropzone;
    const onUploadDone = ({file}: {file: any}) => {
      if (file._giteaUploadId === curUploadId) {
        dropzoneInst.off(DropzoneCustomEventUploadDone, onUploadDone);
        resolve(file);
      }
    };
    dropzoneInst.on(DropzoneCustomEventUploadDone, onUploadDone);
    // FIXME: this is not entirely correct because `file` does not satisfy DropzoneFile (we have abused the Dropzone for long time)
    dropzoneInst.addFile(file as DropzoneFile);
    if ((file as DropzoneFile).status === 'error') {
      removePlaceholder();
    }
  });
}

class TextareaEditor {
  editor: HTMLTextAreaElement;

  constructor(editor: HTMLTextAreaElement) {
    this.editor = editor;
  }

  insertPlaceholder(value: string) {
    replaceTextareaSelection(this.editor, value);
  }

  replacePlaceholder(oldVal: string, newVal: string) {
    const editor = this.editor;
    const startPos = editor.selectionStart;
    const endPos = editor.selectionEnd;
    if (editor.value.substring(startPos, endPos) === oldVal) {
      editor.value = editor.value.substring(0, startPos) + newVal + editor.value.substring(endPos);
      editor.selectionEnd = startPos + newVal.length;
    } else {
      editor.value = editor.value.replace(oldVal, newVal);
      editor.selectionEnd -= oldVal.length;
      editor.selectionEnd += newVal.length;
    }
    editor.selectionStart = editor.selectionEnd;
    const selPos = endPos + newVal.length - oldVal.length;
    editor.setSelectionRange(selPos, selPos);
    editor.focus();
    triggerEditorContentChanged(editor);
  }
}

class CodeMirrorEditor {
  editor: CodeMirror.EditorFromTextArea;

  constructor(editor: CodeMirror.EditorFromTextArea) {
    this.editor = editor;
  }

  insertPlaceholder(value: string) {
    const editor = this.editor;
    const startPoint = editor.getCursor('start');
    const endPoint = editor.getCursor('end');
    editor.replaceSelection(value);
    endPoint.ch = startPoint.ch + value.length;
    editor.setSelection(startPoint, endPoint);
    editor.focus();
    triggerEditorContentChanged(editor.getTextArea());
  }

  replacePlaceholder(oldVal: string, newVal: string) {
    const editor = this.editor;
    const endPoint = editor.getCursor('end');
    if (editor.getSelection() === oldVal) {
      editor.replaceSelection(newVal);
    } else {
      editor.setValue(editor.getValue().replace(oldVal, newVal));
    }
    endPoint.ch -= oldVal.length;
    endPoint.ch += newVal.length;
    editor.setSelection(endPoint, endPoint);
    editor.focus();
    triggerEditorContentChanged(editor.getTextArea());
  }
}

async function handleUploadFiles(editor: CodeMirrorEditor | TextareaEditor, dropzoneEl: HTMLElement, files: Array<File> | FileList, e: Event) {
  e.preventDefault();
  for (const fileOrig of files) {
    let file = fileOrig;
    const name = file.name.slice(0, file.name.lastIndexOf('.'));
    const {width, dppx} = await imageInfo(file);
    const placeholder = `[${name}](uploading ...)`;

    if (document.querySelector('#issue-title-display') && file.size > 10240 && !isImageFile(file) && !isVideoFile(file) && !isPDFFile(file) && !isCompressedFile(file)) {
      file = await compressFileToZip(file);
    }

    editor.insertPlaceholder(placeholder);

    const removePlaceholder = () => {
      editor.replacePlaceholder(placeholder, '');
    };

    await uploadFile(dropzoneEl, file, removePlaceholder); // the "file" will get its "uuid" during the upload
    editor.replacePlaceholder(placeholder, generateMarkdownLinkForAttachment(file, {width, dppx}));
  }
}

export function removeAttachmentLinksFromMarkdown(text: string, fileUuid: string) {
  text = text.replace(new RegExp(`!?\\[([^\\]]+)\\]\\(/?attachments/${fileUuid}\\)`, 'g'), '');
  text = text.replace(new RegExp(`[<]img[^>]+src="/?attachments/${fileUuid}"[^>]*>`, 'g'), '');
  return text;
}

function getPastedFiles(e: ClipboardEvent) {
  const files: Array<File> = [];
  for (const item of e.clipboardData?.items ?? []) {
    const file = item.getAsFile();
    if (file) {
      files.push(file);
    }
  }
  return files;
}

export function initEasyMDEPaste(easyMDE: EasyMDE, dropzoneEl: HTMLElement) {
  const editor = new CodeMirrorEditor(easyMDE.codemirror as any);
  easyMDE.codemirror.on('paste', (_, e) => {
    const files = getPastedFiles(e);
    if (!files.length) return;
    handleUploadFiles(editor, dropzoneEl, files, e);
  });
  easyMDE.codemirror.on('drop', (_, e) => {
    if (!e.dataTransfer?.files.length) return;
    handleUploadFiles(editor, dropzoneEl, e.dataTransfer.files, e);
  });
  easyMDE.codemirror.on('blur', () => {
    window.lastSelection = {
      start: easyMDE.codemirror.indexFromPos(easyMDE.codemirror.getCursor('start')),
      end: easyMDE.codemirror.indexFromPos(easyMDE.codemirror.getCursor('end')),
    };
  });
  dropzoneEl.dropzone.on(DropzoneCustomEventRemovedFile, ({fileUuid}) => {
    const oldText = easyMDE.codemirror.getValue();
    const newText = removeAttachmentLinksFromMarkdown(oldText, fileUuid);
    if (oldText !== newText) easyMDE.codemirror.setValue(newText);
  });
}

export function initTextareaEvents(textarea: HTMLTextAreaElement, dropzoneEl: HTMLElement | null) {
  subscribe(textarea); // enable paste features
  textarea.addEventListener('paste', (e: ClipboardEvent) => {
    const files = getPastedFiles(e);
    if (files.length && dropzoneEl) {
      handleUploadFiles(new TextareaEditor(textarea), dropzoneEl, files, e);
    }
  });
  textarea.addEventListener('drop', (e: DragEvent) => {
    if (!e.dataTransfer?.files.length) return;
    if (!dropzoneEl) return;
    handleUploadFiles(new TextareaEditor(textarea), dropzoneEl, e.dataTransfer.files, e);
  });
  textarea.addEventListener('blur', () => {
    window.lastSelection = {
      start: textarea.selectionStart,
      end: textarea.selectionEnd,
    };
  });
  textarea.addEventListener('keydown', (e) => {
    const isCopy = (e.ctrlKey || e.metaKey) && e.key === 'c';
    const isCut = (e.ctrlKey || e.metaKey) && e.key === 'x';
    if (isCopy || isCut) {
      const start = textarea.selectionStart;
      const end = textarea.selectionEnd;
      if (start !== end) return;
      e.preventDefault();
      const lineStart = textarea.value.lastIndexOf('\n', start - 1) + 1;
      const lineEnd = textarea.value.indexOf('\n', end);
      const realEnd = lineEnd === -1 ? textarea.value.length : lineEnd + 1;
      const block = textarea.value.slice(lineStart, realEnd);
      navigator.clipboard.writeText(block);
      if (isCut) {
        textarea.value = textarea.value.slice(0, lineStart) + textarea.value.slice(realEnd);
        textarea.setSelectionRange(lineStart, lineStart);
      }
      return;
    }

    if (e.key !== 'Tab') return;
    e.preventDefault();

    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const value = textarea.value;

    const lineStart = value.lastIndexOf('\n', start - 1) + 1;
    const lineEnd = value.indexOf('\n', end);
    const realEnd = lineEnd === -1 ? value.length : lineEnd;

    const block = value.slice(lineStart, realEnd);

    if (start === end) {
      if (e.shiftKey) {
        if (value[start - 1] === '\t') {
          textarea.value = value.slice(0, start - 1) + value.slice(start);
          textarea.setSelectionRange(start - 1, start - 1);
        }
      } else {
        textarea.value = `${value.slice(0, start)}\t${value.slice(end)}`;
        textarea.setSelectionRange(start + 1, start + 1);
      }
      return;
    }

    const lines = block.split('\n');
    const newLines = lines.map((line) => {
      if (e.shiftKey) {
        return line.startsWith('\t') ? line.slice(1) : line;
      }
      return `\t${line}`;
    });

    const indented = newLines.join('\n');
    textarea.value = value.slice(0, lineStart) + indented + value.slice(realEnd);
    let newEnd = end + indented.length - block.length;

    if (e.shiftKey) {
      newEnd = end - lines.filter((line) => line.startsWith('\t')).length;
    }

    textarea.setSelectionRange(start, newEnd);
  });
  dropzoneEl?.dropzone.on(DropzoneCustomEventRemovedFile, ({fileUuid}: {fileUuid: string}) => {
    const newText = removeAttachmentLinksFromMarkdown(textarea.value, fileUuid);
    if (textarea.value !== newText) textarea.value = newText;
  });
}
