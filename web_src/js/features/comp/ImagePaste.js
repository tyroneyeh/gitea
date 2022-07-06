import $ from 'jquery';

/**
 *
 * @param {*} editor
 * @param {*} file
 */
export function addUploadedFileToEditor(editor, file) {
  const startPos = editor.selectionStart || editor.getCursor && editor.getCursor('start'), endPos = editor.selectionEnd || editor.getCursor && editor.getCursor('end'), isimage = file.type.startsWith('image/') ? '!' : '', fileName = file.name.replace(/\.[^/.]+$/, '');
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
  if (editor) {
    if (editor.setValue) {
      editor.setValue(editor.getValue().replace(re, '')); // at the moment, we assume the editor is an EasyMDE
    } else {
      editor.value = editor.value.replace(re, '');
    }
  } else {
    $('.CodeMirror').each((_, i) => {
      if (i.CodeMirror) {
        i.CodeMirror.setValue(i.CodeMirror.getValue().replace(re, ''));
      }
    });
  }
}

function clipboardPastedImages(e) {
  if (!e.clipboardData && !e.dataTransfer) return [];

  const files = [];
  if (e.clipboardData) {
    for (const item of e.clipboardData.items || []) {
      const file = item.getAsFile();
      // if (!item.type || !item.type.startsWith('image/')) continue;
      if (file === null || !item.type) continue;
      files.push(item.getAsFile());
    }
  } else if (e.dataTransfer) {
    for (const item of e.dataTransfer.files || []) {
      if (!item.type) continue;
      files.push(item);
    }
  }
  return files;
}

export function initEasyMDEImagePaste(easyMDE, $dropzone) {
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
