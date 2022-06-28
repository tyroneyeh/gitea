const {csrfToken} = window.config;

async function uploadFile(file, uploadUrl, dropzone) {
  const formData = new FormData();
  formData.append('file', file, file.name);

  const res = await fetch(uploadUrl, {
    method: 'POST',
    headers: {'X-Csrf-Token': csrfToken},
    body: formData,
  });
  const data = await res.json();
  const upfile = {name: file.name, size: file.size, uuid: data.uuid};
  dropzone.dropzone.emit('addedfile', upfile);
  dropzone.dropzone.emit('thumbnail', upfile, `/attachments/${data.uuid}`);
  dropzone.dropzone.emit('complete', upfile);
  dropzone.dropzone.files.push(upfile);
  return data;
}

function clipboardPastedImages(e) {
  if (!e.clipboardData) return [];

  const files = [];
  for (const item of e.clipboardData.items || []) {
    if (!item.type || !item.type.startsWith('image/')) continue;
    files.push(item.getAsFile());
  }

  if (files.length) {
    e.preventDefault();
    e.stopPropagation();
  }
  return files;
}


function insertAtCursor(field, value) {
  const startPos = field._data_easyMDE.codemirror.getCursor();
  if (startPos) {
    field._data_easyMDE.codemirror.setSelection(startPos, startPos);
    field._data_easyMDE.codemirror.replaceSelection(value);
  } else {
    field._data_easyMDE.value(field.value + value);
  }
}

function replaceAndKeepCursor(field, oldval, newval) {
  const startPos = field._data_easyMDE.codemirror.getCursor();
  field._data_easyMDE.value(field.value.replace(oldval, newval));
  if (startPos) {
    startPos.line += 1;
    field._data_easyMDE.codemirror.setCursor(startPos);
  }
}

export function initCompImagePaste($target) {
  const dropzone = $target[0].querySelector('.dropzone');
  if (!dropzone) {
    return;
  }
  const uploadUrl = dropzone.getAttribute('data-upload-url');
  const dropzoneFiles = dropzone.querySelector('.files');
  $(document).on('paste', '.CodeMirror', async function(e) {
    const $editor = this.CodeMirror.getTextArea();
    // const img = clipboardPastedImages(e.originalEvent);
    for (const img of clipboardPastedImages(e.originalEvent)) {
      const name = img.name.substring(0, img.name.lastIndexOf('.'));
      insertAtCursor($editor, `![${name}]()`);
      const data = await uploadFile(img, uploadUrl, dropzone);
      replaceAndKeepCursor($editor, `![${name}]()`, `![${name}](/attachments/${data.uuid})\n`);
      const input = $(`<input id="${data.uuid}" name="files" type="hidden">`).val(data.uuid);
      dropzoneFiles.appendChild(input[0]);
    }
  });
}

export function initEasyMDEImagePaste(easyMDE, dropzone, files) {
  const uploadUrl = dropzone.getAttribute('data-upload-url');
  easyMDE.codemirror.on('paste', async (_, e) => {
    for (const img of clipboardPastedImages(e)) {
      const name = img.name.substr(0, img.name.lastIndexOf('.'));
      const data = await uploadFile(img, uploadUrl, dropzone);
      const pos = easyMDE.codemirror.getCursor();
      easyMDE.codemirror.replaceRange(`![${name}](/attachments/${data.uuid})`, pos);
      const input = $(`<input id="${data.uuid}" name="files" type="hidden">`).val(data.uuid);
      files.append(input);
    }
  });
}
