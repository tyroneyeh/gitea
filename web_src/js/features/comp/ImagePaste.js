import $ from 'jquery';
import {getAttachedEasyMDE} from './EasyMDE.js';

/**
 *
 * @param {*} editor
 * @param {*} file
 */
export function addUploadedFileToEditor(editor, file) {
  if (!editor && file.previewElement && (editor = getAttachedEasyMDE(file.previewElement.closest('form').querySelector('textarea')))) {
    editor = editor.codemirror;
  }
  const startPos = editor.selectionStart || editor.getCursor && editor.getCursor('start');
  const endPos = editor.selectionEnd || editor.getCursor && editor.getCursor('end');
  const isimage = file.type.startsWith('image/') ? '!' : '';
  const fileName = (isimage ? file.name.replace(/\.[^/.]+$/, '') : file.name);
  if (startPos) {
    if (editor.setSelection) {
      editor.setSelection(startPos, endPos);
      editor.replaceSelection(`${isimage}[${fileName}](/attachments/${file.uuid})\n`);
    } else {
      editor.value = `${editor.value.substring(0, startPos)}\n${isimage}[${fileName}](/attachments/${file.uuid})\n${editor.value.substring(endPos)}`;
    }
  } else if (editor.setSelection) {
    editor.value(`${editor.value()}\n${isimage}[${fileName}](/attachments/${file.uuid})\n`);
  } else {
    editor.value += `${editor.value}\n${isimage}[${fileName}](/attachments/${file.uuid})\n`;
  }
}

/**
 * @param editor{EasyMDE}
 * @param fileUuid
 */
export function removeUploadedFileFromEditor(editor, fileUuid) {
  // the raw regexp is: /!\[[^\]]*]\(\/attachments\/{uuid}\)/
  const re = new RegExp(`(!|)\\[[^\\]]*]\\(/attachments/${fileUuid}\\)`);
  if (editor.setValue) {
    editor.setValue(editor.getValue().replace(re, '')); // at the moment, we assume the editor is an EasyMDE
  } else {
    editor.value = editor.value.replace(re, '');
  }
}

function clipboardPastedImages(e) {
  const data = e.clipboardData || e.dataTransfer;
  if (!data) return [];

  const files = [];
  const datafiles = e.clipboardData && e.clipboardData.items || e.dataTransfer && e.dataTransfer.files;
  for (const item of datafiles || []) {
    const file = e.clipboardData ? item.getAsFile() : item;
    if (file === null || !item.type) continue;
    files.push(file);
  }
  return files;
}

export function initEasyMDEImagePaste(easyMDE, $dropzone) {
  if ($dropzone.length !== 1) throw new Error('invalid dropzone binding for editor');

  const uploadUrl = $dropzone.attr('data-upload-url');
  const $files = $dropzone.find('.files');

  if (!uploadUrl || !$files.length) return;

  const uploadClipboardImage = async (editor, e) => {
    const pastedImages = clipboardPastedImages(e);
    if (!pastedImages || pastedImages.length === 0) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();

    for (const img of pastedImages) {
      img.editor = editor;
      $dropzone[0].dropzone.addFile(img);
    }
  };

  easyMDE.codemirror.on('paste', async (_, e) => {
    return uploadClipboardImage(easyMDE.codemirror, e);
  });

  easyMDE.codemirror.on('drop', async (_, e) => {
    return uploadClipboardImage(easyMDE.codemirror, e);
  });

  $(easyMDE.element).on('paste drop', async (e) => {
    return uploadClipboardImage(easyMDE.element, e.originalEvent);
  });
}

export function AutoJSZip(file) {
  if (file.done || (!/.(bak|cfg|htm|html|json|log|pcap|pcapng|sql|tar|txt|xml)$/.test(file.name) || (/.(json|txt|xml)$/.test(file.name) && file.size < 1000000))) {
    return;
  }
  document.body.style.cursor = 'wait';
  $('.CodeMirror-lines').css('cursor', 'wait');
  const dz = this;
  dz.removeFile(file);
  const z = new JSZip();
  z.file(file.name, file);
  z.generateAsync({
    type: 'blob',
    compression: 'DEFLATE',
    compressionOptions: {level: 9}
  }).then((content) => {
    const f = new File([content], `${file.name}.zip`);
    f.done = true;
    dz.addFile(f);
    document.body.style.cursor = 'default';
    $('.CodeMirror-lines').css('cursor', 'default');
  });
}
