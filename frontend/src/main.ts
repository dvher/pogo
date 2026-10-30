import { mount } from 'svelte'
import './app.css'
import NoteView from './NoteView.svelte'
import Manager from './Manager.svelte'
import PrintView from './PrintView.svelte'

const params = new URLSearchParams(location.search)
const target = document.getElementById('app')!
const noteId = params.get('note')
const printId = params.get('print')

if (printId) {
  document.body.classList.add('print-window')
  mount(PrintView, { target, props: { id: printId } })
} else if (noteId) {
  document.body.classList.add('note-window')
  mount(NoteView, { target, props: { id: noteId } })
} else {
  document.body.classList.add('manager-window')
  mount(Manager, { target })
}
