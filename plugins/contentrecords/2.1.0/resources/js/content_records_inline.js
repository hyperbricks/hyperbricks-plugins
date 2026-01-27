(() => {
  const targets = Array.from(document.querySelectorAll("[data-cr-bind][data-cr-id]"));
  if (targets.length === 0) {
    return;
  }

  document.documentElement.classList.add("cr-inline-active");

  let editor = null;
  let overlay = null;

  function closeEditor() {
    if (overlay) {
      overlay.classList.remove("cr-inline-overlay--open");
      window.setTimeout(() => {
        if (overlay) {
          overlay.remove();
          overlay = null;
          editor = null;
        }
      }, 180);
      return;
    }
    if (editor) {
      editor.remove();
      editor = null;
    }
  }

  function buildRequestUrl() {
    return window.location.href;
  }

  function openEditor(target) {
    closeEditor();

    const bind = target.dataset.crBind || "";
    const recordId = target.dataset.crId || "";
    const type = (target.dataset.crType || "text").toLowerCase();
    const current = target.dataset.crValue || "";

    const panel = document.createElement("div");
    panel.className = "cr-inline-editor";
    panel.addEventListener("click", (event) => {
      event.stopPropagation();
    });

    const backdrop = document.createElement("div");
    backdrop.className = "cr-inline-overlay";
    backdrop.addEventListener("click", () => {
      closeEditor();
    });
    backdrop.appendChild(panel);

    const header = document.createElement("div");
    header.className = "cr-inline-editor__header";

    const title = document.createElement("div");
    title.className = "cr-inline-editor__title";
    title.textContent = bind ? `Edit ${bind}` : "Edit";

    const closeBtn = document.createElement("button");
    closeBtn.type = "button";
    closeBtn.className = "cr-inline-editor__close";
    closeBtn.textContent = "Close";
    closeBtn.addEventListener("click", () => {
      closeEditor();
    });

    header.appendChild(title);
    header.appendChild(closeBtn);
    panel.appendChild(header);

    const field = document.createElement("div");
    field.className = "cr-inline-editor__field";
    panel.appendChild(field);

    const label = document.createElement("div");
    label.className = "cr-inline-editor__label";
    label.textContent = type === "image" ? "Upload image" : "Value";
    field.appendChild(label);

    let input = null;
    let fileInput = null;
    let pathInput = null;

    if (type === "markdown") {
      input = document.createElement("textarea");
      input.className = "cr-inline-editor__input cr-inline-editor__input--markdown";
      input.value = current;
      field.appendChild(input);
    } else if (type === "image") {
      fileInput = document.createElement("input");
      fileInput.type = "file";
      fileInput.className = "cr-inline-editor__input";
      fileInput.accept = "image/*";
      field.appendChild(fileInput);

      const pathLabel = document.createElement("div");
      pathLabel.className = "cr-inline-editor__label";
      pathLabel.textContent = "Or set image path";
      field.appendChild(pathLabel);

      pathInput = document.createElement("input");
      pathInput.type = "text";
      pathInput.className = "cr-inline-editor__input";
      pathInput.value = current;
      field.appendChild(pathInput);
    } else {
      input = document.createElement("input");
      input.type = "text";
      input.className = "cr-inline-editor__input";
      input.value = current;
      field.appendChild(input);
    }

    const actions = document.createElement("div");
    actions.className = "cr-inline-editor__actions";

    const cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.className = "ghost";
    cancelBtn.textContent = "Cancel";
    cancelBtn.addEventListener("click", () => {
      closeEditor();
    });

    const saveBtn = document.createElement("button");
    saveBtn.type = "button";
    saveBtn.textContent = "Save";

    const status = document.createElement("div");
    status.className = "cr-inline-editor__status";

    saveBtn.addEventListener("click", async () => {
      if (!bind || !recordId) {
        status.textContent = "Missing record details.";
        return;
      }
      saveBtn.disabled = true;
      status.textContent = "Saving...";

      try {
        const url = buildRequestUrl();
        if (type === "image" && fileInput && fileInput.files && fileInput.files.length > 0) {
          const formData = new FormData();
          formData.append("cr_inline", "1");
          formData.append("record_id", recordId);
          formData.append("bind", bind);
          formData.append("file", fileInput.files[0]);

          const response = await fetch(url, {
            method: "POST",
            body: formData,
            credentials: "same-origin",
          });

          if (!response.ok) {
            throw new Error(await response.text());
          }
        } else {
          const value = type === "image" && pathInput ? pathInput.value : input ? input.value : "";
          const response = await fetch(url, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
            },
            body: JSON.stringify({
              cr_inline: "1",
              record_id: recordId,
              bind: bind,
              value: value,
            }),
            credentials: "same-origin",
          });

          if (!response.ok) {
            throw new Error(await response.text());
          }
        }

        window.location.reload();
      } catch (err) {
        status.textContent = "Save failed. See console.";
        saveBtn.disabled = false;
        console.error(err);
      }
    });

    actions.appendChild(cancelBtn);
    actions.appendChild(saveBtn);
    panel.appendChild(actions);
    panel.appendChild(status);

    document.body.appendChild(backdrop);
    overlay = backdrop;
    editor = panel;

    requestAnimationFrame(() => {
      if (overlay) {
        overlay.classList.add("cr-inline-overlay--open");
      }
    });

    if (input) {
      input.focus();
      input.select();
    } else if (pathInput) {
      pathInput.focus();
      pathInput.select();
    } else if (fileInput) {
      fileInput.focus();
    }
  }

  document.addEventListener("click", (event) => {
    if (event.target.closest(".cr-inline-editor")) {
      return;
    }
    const target = event.target.closest("[data-cr-bind][data-cr-id]");
    if (!target) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    openEditor(target);
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeEditor();
    }
  });
})();
