import { mount } from 'svelte'
import './app.css'
import NoteView from './NoteView.svelte'
import Manager from './Manager.svelte'

const params = new URLSearchParams(location.search)
const target = document.getElementById('app')!
const noteId = params.get('note')

if (noteId) {
  document.body.classList.add('note-window')
  mount(NoteView, { target, props: { id: noteId } })
} else {
  document.body.classList.add('manager-window')
  mount(Manager, { target })
}
