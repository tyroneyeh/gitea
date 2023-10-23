import $ from 'jquery';
function getComboMarkdownEditor(el) {
  if (el instanceof $) el = el[0];
  return el?._giteaComboMarkdownEditor;
}

function clipboardPastedImages(e) {
  const datafiles = e.clipboardData && e.clipboardData.items || e.dataTransfer && e.dataTransfer.files;
  if (!datafiles) return [];

  const files = [];
  for (const item of datafiles || []) {
    const file = e.clipboardData ? item.getAsFile() : item;
    if (!file) continue;
    files.push(file);
  }
  return files;
}

function triggerEditorContentChanged(target) {
  target.dispatchEvent(new CustomEvent('ce-editor-content-changed', {bubbles: true}));
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
    editor.selectionStart = startPos;
    editor.selectionEnd = startPos + value.length;
    editor.focus();
    triggerEditorContentChanged(editor);
  }

  replacePlaceholder(oldVal, newVal) {
    const editor = this.editor;
    const startPos = editor.selectionStart;
    const endPos = editor.selectionEnd;
    if (typeof editor.value === 'string') {
      if (editor.value.substring(startPos, endPos) === oldVal) {
        editor.value = editor.value.substring(0, startPos) + newVal + editor.value.substring(endPos);
        editor.selectionEnd = startPos + newVal.length;
      } else {
        editor.value = editor.value.replace(oldVal, newVal);
        editor.selectionEnd -= oldVal.length;
        editor.selectionEnd += newVal.length;
      }
      triggerEditorContentChanged(editor);
    } else {
      const value = editor.value();
      if (startPos !== undefined && value.substring(startPos, endPos) === oldVal) {
        editor.value(value.substring(0, startPos) + newVal + value.substring(endPos));
        editor.selectionEnd = startPos + newVal.length;
      } else {
        editor.value(value.replace(oldVal, newVal));
        editor.selectionEnd -= oldVal.length;
        editor.selectionEnd += newVal.length;
      }
    }

    editor.selectionStart = editor.selectionEnd;
    editor.focus();
  }
}

class CodeMirrorEditor {
  constructor(editor) {
    this.editor = editor;
  }

  insertPlaceholder(value) {
    const editor = this.editor;
    const startPoint = editor.getCursor('start');
    const endPoint = editor.getCursor('end');
    editor.replaceSelection(value);
    endPoint.ch = startPoint.ch + value.length;
    editor.setSelection(startPoint, endPoint);
    editor.focus();
    triggerEditorContentChanged(editor.getTextArea());
  }

  replacePlaceholder(oldVal, newVal) {
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


const uploadClipboardImage = async (editor, dropzone, e) => {
  const $dropzone = $(dropzone);
  const uploadUrl = $dropzone.attr('data-upload-url');
  const $files = $dropzone.find('.files');

  if (!uploadUrl || !$files.length) return;

  const pastedImages = clipboardPastedImages(e);
  if (!pastedImages || pastedImages.length === 0) {
    return;
  }
  e.preventDefault();
  e.stopPropagation();

  for (const img of pastedImages) {
    const name = img.name.slice(0, img.name.lastIndexOf('.'));
    const imgSymbol = img.type.includes('image') ? '!' : '';

    const placeholder = `${imgSymbol}[${name}](uploading ...)`;
    editor.insertPlaceholder(placeholder);

    if (addUploadedFileToEditor($dropzone, img)) {
      img.done = true;
    }
  }
};

export function initEasyMDEImagePaste(easyMDE, dropzone) {
  if (!dropzone) return;
  easyMDE.codemirror.on('paste drop', async (_, e) => {
    return uploadClipboardImage(new CodeMirrorEditor(easyMDE.codemirror), dropzone, e);
  });
}

export function initTextareaImagePaste(textarea, dropzone) {
  if (!dropzone) return;
  $(textarea).on('paste drop', async (e) => {
    return uploadClipboardImage(new TextareaEditor(textarea), dropzone, e.originalEvent);
  });
}

/**
 * @returns {JSZip}
 */
async function importJSZip() {
  const {default: JSZip} = await import('jszip');
  return JSZip;
}

async function addUploadedFileToEditor($dropzone, file) {
  if (!file.editor) {
    const form = $dropzone.closest('div.comment,form.form')[0];
    if (form) {
      const editor = getComboMarkdownEditor(form.querySelector('textarea'));
      if (editor) {
        if (editor.codemirror) {
          file.editor = new CodeMirrorEditor(editor.codemirror);
        } else {
          file.editor = new TextareaEditor(editor);
        }
      }
    }
  }
  if (file.done || (file.type.includes('application') || (file.type.includes('text') && file.size < 1000000))) {
    $dropzone[0].dropzone.addFile(file);
    return true;
  }

  document.body.style.cursor = 'wait';
  $('.markdown-text-editor').css('cursor', 'wait');
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
    // $dropzone[0].dropzone.removeFile(file);
    $dropzone[0].dropzone.addFile(f);
    document.body.style.cursor = 'default';
    $('.markdown-text-editor').css('cursor', 'default');
  });
}

