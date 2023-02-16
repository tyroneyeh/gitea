import $ from 'jquery';
import {getAttachedEasyMDE} from './EasyMDE.js';

/**
 * @param editor{EasyMDE}
 * @param fileUuid
 */
export function removeUploadedFileFromEditor(editor, fileUuid) {
  // the raw regexp is: /!\[[^\]]*]\(\/attachments\/{uuid}\)/
  if (editor && editor.editor) {
    const re = new RegExp(`(!|)\\[[^\\]]*]\\(/attachments/${fileUuid}\\)`);
    if (editor.editor.setValue) {
      editor.editor.setValue(editor.editor.getValue().replace(re, '')); // at the moment, we assume the editor is an EasyMDE
    } else {
      editor.editor.value = editor.editor.value.replace(re, '');
    }
  }
}

function clipboardPastedFiles(e) {
  const data = e.clipboardData || e.dataTransfer;
  if (!data) return [];

  const files = [];
  const datafiles = e.clipboardData && e.clipboardData.items || e.dataTransfer && e.dataTransfer.files;
  for (const item of datafiles || []) {
    const file = e.clipboardData ? item.getAsFile() : item;
    if (file === null) continue;
    files.push(file);
  }
  return files;
}

class TextareaEditor {
  constructor(editor) {
    this.editor = editor;
  }

  insertPlaceholder(value) {
    const editor = this.editor;
    const startPos = editor.selectionStart;
    const endPos = editor.selectionEnd;
    editor.value = editor.value.substring(0, startPos) + value + editor.value.substring(endPos);
    // editor.selectionStart = startPos;
    // editor.selectionEnd = startPos + value.length;
    editor.focus();
  }

  // replacePlaceholder(oldVal, newVal) {
  //   const editor = this.editor;
  //   const startPos = editor.selectionStart;
  //   const endPos = editor.selectionEnd;
  //   if (editor.value.substring(startPos, endPos) === oldVal) {
  //     editor.value = editor.value.substring(0, startPos) + newVal + editor.value.substring(endPos);
  //     editor.selectionEnd = startPos + newVal.length;
  //   } else {
  //     editor.value = editor.value.replace(oldVal, newVal);
  //     editor.selectionEnd -= oldVal.length;
  //     editor.selectionEnd += newVal.length;
  //   }
  //   editor.selectionStart = editor.selectionEnd;
  //   editor.focus();
  // }
}

class CodeMirrorEditor {
  constructor(editor) {
    this.editor = editor;
  }

  insertPlaceholder(value) {
    const editor = this.editor;
    // const startPoint = editor.getCursor('start');
    // const endPoint = editor.getCursor('end');
    editor.replaceSelection(value);
    // endPoint.ch = startPoint.ch + value.length;
    // editor.setSelection(startPoint, endPoint);
    editor.focus();
  }

  // replacePlaceholder(oldVal, newVal) {
  //   const editor = this.editor;
  //   const endPoint = editor.getCursor('end');
  //   if (editor.getSelection() === oldVal) {
  //     editor.replaceSelection(newVal);
  //   } else {
  //     editor.setValue(editor.getValue().replace(oldVal, newVal));
  //   }
  //   endPoint.ch -= oldVal.length;
  //   endPoint.ch += newVal.length;
  //   editor.setSelection(endPoint, endPoint);
  //   editor.focus();
  // }
}

export function initEasyMDEFilePaste(easyMDE, $dropzone) {
  // if ($dropzone.length !== 1) throw new Error('invalid dropzone binding for editor');

  const uploadUrl = $dropzone.attr('data-upload-url');
  const $files = $dropzone.find('.files');

  if (!uploadUrl || !$files.length) return;

  const uploadClipboardFile = async (editor, e) => {
    const pastedFiles = clipboardPastedFiles(e);
    if (!pastedFiles || pastedFiles.length === 0) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();

    for (const file of pastedFiles) {
      file.editor = editor;
      if (addUploadedFileToEditor($dropzone, file)) {
        file.done = true;
        $dropzone[0].dropzone.addFile(file);
      }
    }
  };

  easyMDE.codemirror.on('paste', async (_, e) => {
    return uploadClipboardFile(new CodeMirrorEditor(easyMDE.codemirror), e);
  });

  easyMDE.codemirror.on('drop', async (_, e) => {
    return uploadClipboardFile(new CodeMirrorEditor(easyMDE.codemirror), e);
  });

  $(easyMDE.element).on('paste drop', async (e) => {
    return uploadClipboardFile(new TextareaEditor(easyMDE.element), e.originalEvent);
  });
}

/**
 * @returns {JSZip}
 */
async function importJSZip() {
  const {default: JSZip} = await import('jszip');
  return JSZip;
}

export async function addUploadedFileToEditor($dropzone, file) {
  if (!file.editor) {
    const form = file.previewElement.closest('div.comment,form.form');
    if (form) {
      const editor = getAttachedEasyMDE(form.querySelector('textarea'));
      if (editor) {
        if (editor.codemirror) {
          file.editor = new CodeMirrorEditor(editor.codemirror);
        } else {
          file.editor = new TextareaEditor(editor);
        }
      }
    }
  }
  if (file.done || (/\.(7z|bz2|gif|gz|jpe?g|mp4|odp|(?:f|)ods|odt|pdf|png|svg|webm|webp|xz|zip)$/i.test(file.name) || (/\.(csv|js|json|htm|html|txt|xml|yma?l)$/i.test(file.name) && file.size < 1000000))) {
    return true;
  }

  document.body.style.cursor = 'wait';
  $('.CodeMirror-lines').css('cursor', 'wait');
  const JSZip = await importJSZip();
  const z = new JSZip();
  z.file(file.name, file);
  z.generateAsync({
    type: 'blob',
    compression: 'DEFLATE',
    compressionOptions: {level: 9}
  }).then((content) => {
    const fileName = file.name.slice(0, file.name.lastIndexOf('.'));
    const f = new File([content], `${fileName}.zip`);
    f.editor = file.editor;
    f.done = true;
    $dropzone[0].dropzone.removeFile(file);
    $dropzone[0].dropzone.addFile(f);
    document.body.style.cursor = 'default';
    $('.CodeMirror-lines').css('cursor', 'default');
  });
}
